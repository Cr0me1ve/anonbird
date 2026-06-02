# NetBird transport analysis for AnonBird

Дата анализа: 2026-05-31

## Existing transport model

Обычный NetBird использует control plane и data plane как отдельные слои:

- Management gRPC получает login/sync, network map, setup key enrollment, account/device metadata.
- Signal gRPC передает offer/answer/candidate messages между пирами.
- Peer connection manager пробует ICE/direct UDP и relay параллельно, затем выбирает лучший путь.
- Relay client умеет подключаться к relay через QUIC/WebSocket и может использовать IP fallback для foreign relay.
- WireGuard endpoint настраивается либо на ICE-discovered UDP endpoint, либо на локальный proxy endpoint для relay.

Эта модель полезна для скорости, но для anonymous mode опасна: она оптимизирована на нахождение прямого пути и публикацию сетевых hints.

## Current connection sequence

1. Клиент читает profile config и создает management gRPC client.
2. Login/Sync отправляет management metadata, включая system info и flags.
3. Management возвращает `NetbirdConfig`: signal URL, relay URLs, STUN/TURN.
4. Клиент создает signal gRPC client и relay manager.
5. Для каждого peer `Conn.Open` создает relay worker и, если не force-relayed, ICE worker.
6. Handshaker слушает relay и ICE offers.
7. ICE path может отправлять candidates через signal и настроить прямой WireGuard UDP endpoint.
8. Relay path открывает relay connection и настраивает WireGuard на локальный relay proxy endpoint.

AnonBird MVP должен сохранить только шаги control plane + relay path, но заменить все внешние dialers на anonymous transport.

## Anonymous MVP transport

Целевой режим `tor-relay-only`:

```text
client gRPC/relay dial
  -> Tor SOCKS5 (default 127.0.0.1:9050)
  -> .onion management/signal/relay
  -> relay session
  -> userspace WireGuard
```

Allowed endpoints:

- `http://*.onion` for management/signal/relay during Tor MVP.
- `http://*.b32.i2p` for I2P SAM-backed management/signal/relay.
- local SOCKS5 address for Tor bootstrap, default `127.0.0.1:9050`.
- local I2P SAM address, default `127.0.0.1:7656`.
- overlay WireGuard IP/DNS inside the mesh.

Forbidden endpoints:

- public DNS names;
- public IPv4/IPv6 addresses;
- STUN/TURN URLs;
- relay `serverIP` fallback;
- ICE host/srflx/relay candidates;
- LAN interface addresses and local hostnames in metadata.

## Required code changes

### Configuration

Add persistent anonymous settings to client profile and daemon API:

- `anonymous_mode`;
- `anonymous_transport.type` with values `tor-relay-only` and `i2p-datagram`;
- `anonymous_transport.require_anonymous`;
- `anonymous_transport.tor_socks5`;
- `anonymous_transport.i2p_sam`.

When `anonymous_mode=true`, config must force or validate:

- management URL is anonymous;
- NAT external IPs are empty;
- client/server routes are disabled for MVP no-exit/no-subnet scope;
- LAN access is blocked;
- overlay inbound must remain allowed so peers can exchange encrypted private-mesh traffic through relay;
- userspace WireGuard is required.

### gRPC over anonymous transport

Management and signal clients need anonymous dial paths:

- normal mode keeps `client/grpc.CreateConnection`;
- anonymous Tor mode uses a SOCKS5 `ContextDialer`;
- anonymous I2P mode uses a SAM STREAM `ContextDialer`;
- TLS/insecure behavior remains tied to URL scheme, but the TCP connection is opened through SOCKS5;
- clearnet fallback is not attempted after SOCKS5/SAM failure.

### Relay over anonymous transport

Relay client needs an anonymous option:

- relay picker constructs clients with SOCKS5 proxy configured;
- relay picker can construct clients with SOCKS5 proxy or I2P SAM configured;
- dialer list in anonymous mode is WebSocket-only;
- QUIC is disabled because it is UDP and cannot use Tor SOCKS or I2P SAM STREAM;
- `NewClientWithServerIP`/foreign relay path must ignore `serverIP` when anonymous transport is active;
- relay URLs are validated as `.onion`/`.b32.i2p` and must match the selected transport.

When `tor-relay-only` is selected, the client first verifies a local loopback
SOCKS5 proxy (default `127.0.0.1:9050`). If it is unavailable, AnonBird tries to
resolve or install the `tor` package and starts a managed Tor client with a
private `torrc`/data directory. Remote SOCKS5 addresses are rejected in
anonymous mode, and management/signal/relay dials still fail closed if the URL
is not an endpoint matching the selected `.onion` transport.

### I2P datagram data plane

The I2P backend now has both the SAM primitive and a direct peer binding for WireGuard traffic:

- generates persistent I2P destination keys with `DEST GENERATE SIGNATURE_TYPE=7`;
- can create SAM sessions from a persistent private destination key instead of only `TRANSIENT`;
- creates `STYLE=RAW` or `STYLE=DATAGRAM` SAM sessions through the control socket;
- uses `SIGNATURE_TYPE=7` and explicit tunnel quantities by default;
- sends payloads through the SAM UDP bridge with the SAM v3 datagram header;
- receives and parses `RAW RECEIVED` / `DATAGRAM RECEIVED` messages from the control socket;
- enforces SAM size limits and rejects reserved raw datagram protocol numbers.

When `i2p-datagram` is selected, the client can use a system SAM bridge (`external`) or manage an `i2pd` process (`auto`/`managed`). The managed path checks for a local SAM bridge, installs the `i2pd` package when the default binary is missing and a supported package manager is available, writes a per-profile `i2pd.conf` with SAM enabled, waits for SAM readiness before login/dial, and stops the owned process when the client shuts down. The public I2P destination is registered through management metadata; the private destination remains in the local profile config.

Operational i2pd requirements, version guidance, systemd expectations and recovery commands are tracked in `docs/anonbird-i2p-operations.md`.

For peer traffic, `Conn.Open` receives the remote peer's public I2P destination from `RemotePeerConfig.anonymousTransport`. It opens a logical `net.Conn` backed by one shared local SAM `STYLE=DATAGRAM` session, so multiple peers reuse the same persistent I2P destination instead of creating duplicate SAM sessions. Incoming datagrams are demultiplexed by source I2P destination and handed to the existing WireGuard proxy as packet reads; outgoing WireGuard packets are sent as SAM datagrams to the remote destination. Anonymous mode still skips ICE worker creation and refuses ICE candidate signaling, so no STUN/direct UDP endpoint is introduced by this binding. Relay-over-I2P remains an anonymous fallback if the direct datagram path is unavailable.

### Peer connection kill-switch

Peer connections need an explicit anonymous flag:

- `ConnConfig.AnonymousMode`;
- `Conn.Open` skips `NewWorkerICE`;
- handshaker registers only relay listener;
- `SignalICECandidate` refuses to send candidates;
- signal credential marshal omits relay server IP fallback.

### Management metadata privacy

The client must send a sanitized `PeerSystemMeta` in anonymous mode:

- stable anonymous hostname derived from local public key, e.g. `anonbird-<hash>`;
- no network addresses;
- no serial number, manufacturer or product name;
- anonymous flag in peer local flags.

Management server must honor that flag:

- ignore realip middleware result for Login/Sync/SyncMeta;
- store nil connection IP;
- omit connection IP in events/API responses;
- expose `anonymous_mode` to dashboard so UI can hide sensitive fields.

### Dashboard

Dashboard should treat anonymous peers as a distinct mode, not as normal peers with missing fields:

- label network mode as `anonymous`;
- show transport `tor-relay-only`;
- show relay connectivity/status;
- show overlay IP/DNS only;
- hide connection IP, endpoint candidates, LAN addresses, public endpoint copy buttons and direct-route controls;
- generate setup commands using `anonbird up --anonymous-mode --management-url ... --setup-key ...`.

## Implementation order

1. Add config/runtime validation and anonymous metadata sanitizer.
2. Add SOCKS5 gRPC dialer and wire management/signal through it.
3. Add relay SOCKS5/WebSocket-only mode.
4. Add peer connection kill-switch for ICE/candidates/direct endpoints.
5. Add management anonymous flag handling and redaction.
6. Update dashboard render paths and setup commands.
7. Run unit tests and then integration checks on disposable servers.

## Integration test outline

The user provided disposable servers:

- `93.177.116.58`
- `213.108.3.228`
- `185.246.220.249`

Use them only after local unit/integration tests pass enough to avoid debugging basic compile errors remotely. Expected deployment:

- one management/signal/relay onion endpoint;
- two client nodes with Tor SOCKS enabled;
- packet capture on clients showing no direct UDP/STUN/ICE to each other;
- management database/API showing empty real connection IP for anonymous peers;
- overlay ping/ssh succeeds through relay.
