package i2psam

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDialStream(t *testing.T) {
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		control := acceptSAM(t, ln)
		defer control.Close()
		requireSAMCommand(t, control, "HELLO VERSION")
		writeSAMReply(t, control, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, control, "SESSION CREATE STYLE=STREAM")
		require.Contains(t, command, "SIGNATURE_TYPE=7")
		require.Contains(t, command, "inbound.length=1")
		require.Contains(t, command, "outbound.length=1")
		require.Contains(t, command, "inbound.quantity=3")
		require.Contains(t, command, "outbound.quantity=3")
		writeSAMReply(t, control, "SESSION STATUS RESULT=OK DESTINATION=fake-destination")

		data := acceptSAM(t, ln)
		defer data.Close()
		requireSAMCommand(t, data, "HELLO VERSION")
		writeSAMReply(t, data, "HELLO REPLY RESULT=OK VERSION=3.3")
		command = requireSAMCommand(t, data, "STREAM CONNECT")
		require.Contains(t, command, "DESTINATION=peer.b32.i2p")
		require.Contains(t, command, "TO_PORT=443")
		writeSAMReply(t, data, "STREAM STATUS RESULT=OK")

		buf := make([]byte, 4)
		_, err := io.ReadFull(data, buf)
		require.NoError(t, err)
		require.Equal(t, "ping", string(buf))
		_, err = data.Write([]byte("pong"))
		require.NoError(t, err)
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := Dialer{SAMAddress: listener.Addr().String(), Timeout: time.Second}.DialContext(ctx, "tcp", "peer.b32.i2p:443")
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	buf := make([]byte, 4)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, "pong", string(buf))
}

func TestDialStreamUsesCustomTunnelSettings(t *testing.T) {
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		control := acceptSAM(t, ln)
		defer control.Close()
		requireSAMCommand(t, control, "HELLO VERSION")
		writeSAMReply(t, control, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, control, "SESSION CREATE STYLE=STREAM")
		require.Contains(t, command, "inbound.length=2")
		require.Contains(t, command, "outbound.length=2")
		require.Contains(t, command, "inbound.quantity=4")
		require.Contains(t, command, "outbound.quantity=4")
		writeSAMReply(t, control, "SESSION STATUS RESULT=OK DESTINATION=fake-destination")

		data := acceptSAM(t, ln)
		defer data.Close()
		requireSAMCommand(t, data, "HELLO VERSION")
		writeSAMReply(t, data, "HELLO REPLY RESULT=OK VERSION=3.3")
		requireSAMCommand(t, data, "STREAM CONNECT")
		writeSAMReply(t, data, "STREAM STATUS RESULT=OK")
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := Dialer{
		SAMAddress:     listener.Addr().String(),
		Timeout:        time.Second,
		TunnelLength:   2,
		TunnelQuantity: 4,
	}.DialContext(ctx, "tcp", "peer.b32.i2p:443")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestDialStreamRejectsInvalidTunnelSettings(t *testing.T) {
	ctx := context.Background()
	_, err := Dialer{TunnelLength: 8}.DialContext(ctx, "tcp", "peer.b32.i2p:443")
	require.Error(t, err)
	require.Contains(t, err.Error(), "tunnel length")

	_, err = Dialer{TunnelQuantity: 17}.DialContext(ctx, "tcp", "peer.b32.i2p:443")
	require.Error(t, err)
	require.Contains(t, err.Error(), "tunnel quantity")
}

func TestDialStreamReturnsSAMFailure(t *testing.T) {
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		control := acceptSAM(t, ln)
		defer control.Close()
		requireSAMCommand(t, control, "HELLO VERSION")
		writeSAMReply(t, control, "HELLO REPLY RESULT=OK VERSION=3.3")
		requireSAMCommand(t, control, "SESSION CREATE STYLE=STREAM")
		writeSAMReply(t, control, "SESSION STATUS RESULT=OK DESTINATION=fake-destination")

		data := acceptSAM(t, ln)
		defer data.Close()
		requireSAMCommand(t, data, "HELLO VERSION")
		writeSAMReply(t, data, "HELLO REPLY RESULT=OK VERSION=3.3")
		requireSAMCommand(t, data, "STREAM CONNECT")
		writeSAMReply(t, data, `STREAM STATUS RESULT=CANT_REACH_PEER MESSAGE="no route"`)
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Dialer{SAMAddress: listener.Addr().String(), Timeout: time.Second}.DialContext(ctx, "tcp", "peer.b32.i2p")
	require.Error(t, err)
	require.Contains(t, err.Error(), "CANT_REACH_PEER")
	require.Contains(t, err.Error(), "no route")
}

func TestLookup(t *testing.T) {
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, conn, "NAMING LOOKUP")
		require.Contains(t, command, "NAME=peer.b32.i2p")
		writeSAMReply(t, conn, "NAMING REPLY RESULT=OK NAME=peer.b32.i2p VALUE=base64-destination")
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	destination, err := Lookup(ctx, listener.Addr().String(), "peer.b32.i2p")
	require.NoError(t, err)
	require.Equal(t, "base64-destination", destination)
}

func TestCheck(t *testing.T) {
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, Check(ctx, listener.Addr().String()))
}

func TestGenerateDestination(t *testing.T) {
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, conn, "DEST GENERATE")
		require.Contains(t, command, "SIGNATURE_TYPE=7")
		writeSAMReply(t, conn, "DEST REPLY PUB=public-destination PRIV=private-destination")
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	destination, err := GenerateDestination(ctx, listener.Addr().String())
	require.NoError(t, err)
	require.Equal(t, "public-destination", destination.Public)
	require.Equal(t, "private-destination", destination.Private)
}

func TestReadReplyParsesQuotedFields(t *testing.T) {
	reply, err := readReply(bufio.NewReader(strings.NewReader(`STREAM STATUS RESULT=I2P_ERROR MESSAGE="router is still building tunnels"` + "\n")))
	require.NoError(t, err)
	require.Equal(t, "I2P_ERROR", reply.Values["RESULT"])
	require.Equal(t, "router is still building tunnels", reply.Values["MESSAGE"])
}

func startFakeSAM(t *testing.T, handler func(t *testing.T, ln net.Listener)) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler(t, listener)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("fake SAM server did not stop")
		}
	})
	return listener
}

func acceptSAM(t *testing.T, listener net.Listener) net.Conn {
	t.Helper()
	conn, err := listener.Accept()
	require.NoError(t, err)
	return conn
}

func requireSAMCommand(t *testing.T, conn net.Conn, prefix string) string {
	t.Helper()
	err := conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	require.NoError(t, err)
	line, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err)
	line = strings.TrimSpace(line)
	require.True(t, strings.HasPrefix(line, prefix), fmt.Sprintf("expected SAM command prefix %q, got %q", prefix, line))
	return line
}

func writeSAMReply(t *testing.T, conn net.Conn, reply string) {
	t.Helper()
	_, err := fmt.Fprintf(conn, "%s\n", reply)
	require.NoError(t, err)
}
