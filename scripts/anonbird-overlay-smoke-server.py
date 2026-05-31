#!/usr/bin/env python3
"""Small HTTP/WebSocket service for AnonBird overlay release tests.

The server intentionally logs no peer addresses. Bind it to an AnonBird overlay
IP to verify that application traffic works through the virtual network and is
not exposed on the host public IP.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import socket
import struct
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


class OverlaySmokeHandler(BaseHTTPRequestHandler):
    server_version = "AnonBirdOverlaySmoke/1.0"

    def log_message(self, fmt: str, *args: object) -> None:
        message = fmt % args
        print(f"{self.log_date_time_string()} {message}", flush=True)

    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        if parsed.path == "/health":
            self.write_json(
                {
                    "status": "ok",
                    "service": "anonbird-overlay-smoke",
                    "bind": self.server.server_address[0],
                    "port": self.server.server_address[1],
                }
            )
            return

        if parsed.path == "/echo":
            query = parse_qs(parsed.query)
            self.write_json({"echo": query.get("message", [""])[0]})
            return

        if parsed.path == "/ws":
            self.handle_websocket()
            return

        self.send_error(HTTPStatus.NOT_FOUND, "not found")

    def write_json(self, payload: dict[str, object]) -> None:
        data = json.dumps(payload, sort_keys=True).encode()
        self.send_response(HTTPStatus.OK)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def handle_websocket(self) -> None:
        key = self.headers.get("Sec-WebSocket-Key")
        if not key:
            self.send_error(HTTPStatus.BAD_REQUEST, "missing websocket key")
            return

        accept = base64.b64encode(hashlib.sha1((key + GUID).encode()).digest()).decode()
        self.send_response(HTTPStatus.SWITCHING_PROTOCOLS)
        self.send_header("Upgrade", "websocket")
        self.send_header("Connection", "Upgrade")
        self.send_header("Sec-WebSocket-Accept", accept)
        self.end_headers()

        while True:
            frame = read_ws_frame(self.connection)
            if frame is None:
                return
            opcode, payload = frame
            if opcode == 0x8:
                write_ws_frame(self.connection, 0x8, b"")
                return
            if opcode == 0x9:
                write_ws_frame(self.connection, 0xA, payload)
                continue
            if opcode == 0x1:
                write_ws_frame(self.connection, 0x1, b"anonbird-ws:" + payload)


def read_exact(conn: socket.socket, size: int) -> bytes | None:
    chunks = []
    remaining = size
    while remaining:
        chunk = conn.recv(remaining)
        if not chunk:
            return None
        chunks.append(chunk)
        remaining -= len(chunk)
    return b"".join(chunks)


def read_ws_frame(conn: socket.socket) -> tuple[int, bytes] | None:
    header = read_exact(conn, 2)
    if header is None:
        return None

    first, second = header
    opcode = first & 0x0F
    masked = bool(second & 0x80)
    length = second & 0x7F

    if length == 126:
        extended = read_exact(conn, 2)
        if extended is None:
            return None
        (length,) = struct.unpack("!H", extended)
    elif length == 127:
        extended = read_exact(conn, 8)
        if extended is None:
            return None
        (length,) = struct.unpack("!Q", extended)

    mask = b""
    if masked:
        mask = read_exact(conn, 4)
        if mask is None:
            return None

    payload = read_exact(conn, length)
    if payload is None:
        return None

    if masked:
        payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))

    return opcode, payload


def write_ws_frame(conn: socket.socket, opcode: int, payload: bytes) -> None:
    first = 0x80 | opcode
    length = len(payload)
    if length < 126:
        header = struct.pack("!BB", first, length)
    elif length < 65536:
        header = struct.pack("!BBH", first, 126, length)
    else:
        header = struct.pack("!BBQ", first, 127, length)
    conn.sendall(header + payload)


def main() -> None:
    bind = os.environ.get("ANONBIRD_SMOKE_BIND", "127.0.0.1")
    port = int(os.environ.get("ANONBIRD_SMOKE_PORT", "18080"))
    server = ThreadingHTTPServer((bind, port), OverlaySmokeHandler)
    print(f"anonbird-overlay-smoke listening on {bind}:{port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
