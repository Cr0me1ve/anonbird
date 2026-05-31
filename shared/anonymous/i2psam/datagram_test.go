package i2psam

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRawDatagramSessionSendReceive(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer udp.Close()

	receivedUDP := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		n, _, err := udp.ReadFrom(buf)
		require.NoError(t, err)
		receivedUDP <- string(buf[:n])
	}()

	sendReply := make(chan struct{})
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, conn, "SESSION CREATE STYLE=RAW")
		require.Contains(t, command, "ID=anonbird-raw-test")
		require.Contains(t, command, "inbound.length=1")
		require.Contains(t, command, "outbound.length=1")
		require.Contains(t, command, "inbound.quantity=3")
		require.Contains(t, command, "outbound.quantity=3")
		require.Contains(t, command, "PROTOCOL=18")
		require.Contains(t, command, "FROM_PORT=777")
		require.Contains(t, command, "TO_PORT=888")
		writeSAMReply(t, conn, "SESSION STATUS RESULT=OK DESTINATION=fake-private-destination")

		<-sendReply
		_, err := fmt.Fprintf(conn, "RAW RECEIVED SIZE=4 FROM_PORT=777 TO_PORT=888 PROTOCOL=18\npong")
		require.NoError(t, err)
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:    listener.Addr().String(),
		SAMUDPAddress: udp.LocalAddr().String(),
		Style:         DatagramStyleRaw,
		SessionID:     "anonbird-raw-test",
		FromPort:      777,
		ToPort:        888,
		Protocol:      DefaultRawProtocol,
	})
	require.NoError(t, err)
	defer session.Close()

	err = session.Send(ctx, "peer.b32.i2p", []byte("ping"), SendOptions{FromPort: 777, ToPort: 888, Protocol: DefaultRawProtocol})
	require.NoError(t, err)

	select {
	case packet := <-receivedUDP:
		require.Equal(t, "3.0 anonbird-raw-test peer.b32.i2p FROM_PORT=777 TO_PORT=888 PROTOCOL=18\nping", packet)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	close(sendReply)
	datagram, err := session.Receive(ctx)
	require.NoError(t, err)
	require.True(t, datagram.AnonymousRaw)
	require.Equal(t, []byte("pong"), datagram.Payload)
	require.Equal(t, uint16(777), datagram.FromPort)
	require.Equal(t, uint16(888), datagram.ToPort)
	require.Equal(t, uint8(DefaultRawProtocol), datagram.Protocol)
}

func TestDatagramSessionUsesCustomTunnelSettings(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer udp.Close()

	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, conn, "SESSION CREATE STYLE=RAW")
		require.Contains(t, command, "inbound.length=2")
		require.Contains(t, command, "outbound.length=3")
		require.Contains(t, command, "inbound.quantity=4")
		require.Contains(t, command, "outbound.quantity=5")
		writeSAMReply(t, conn, "SESSION STATUS RESULT=OK DESTINATION=fake-private-destination")
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:       listener.Addr().String(),
		SAMUDPAddress:    udp.LocalAddr().String(),
		Style:            DatagramStyleRaw,
		SessionID:        "anonbird-custom-tunnel-test",
		InboundLength:    2,
		OutboundLength:   3,
		InboundQuantity:  4,
		OutboundQuantity: 5,
	})
	require.NoError(t, err)
	require.NoError(t, session.Close())
}

func TestRepliableDatagramSessionReceive(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer udp.Close()

	sendReply := make(chan struct{})
	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, conn, "SESSION CREATE STYLE=DATAGRAM")
		require.NotContains(t, command, "PROTOCOL=")
		writeSAMReply(t, conn, "SESSION STATUS RESULT=OK DESTINATION=fake-private-destination")

		<-sendReply
		_, err := fmt.Fprintf(conn, "DATAGRAM RECEIVED DESTINATION=source-destination SIZE=5 FROM_PORT=10 TO_PORT=11\nhello")
		require.NoError(t, err)
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:    listener.Addr().String(),
		SAMUDPAddress: udp.LocalAddr().String(),
		Style:         DatagramStyleRepliable,
		SessionID:     "anonbird-datagram-test",
	})
	require.NoError(t, err)
	defer session.Close()

	close(sendReply)
	datagram, err := session.Receive(ctx)
	require.NoError(t, err)
	require.False(t, datagram.AnonymousRaw)
	require.Equal(t, "source-destination", datagram.Source)
	require.Equal(t, []byte("hello"), datagram.Payload)
	require.Equal(t, uint16(10), datagram.FromPort)
	require.Equal(t, uint16(11), datagram.ToPort)
}

func TestDatagramSessionUsesPrivateDestination(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer udp.Close()

	listener := startFakeSAM(t, func(t *testing.T, ln net.Listener) {
		conn := acceptSAM(t, ln)
		defer conn.Close()
		requireSAMCommand(t, conn, "HELLO VERSION")
		writeSAMReply(t, conn, "HELLO REPLY RESULT=OK VERSION=3.3")
		command := requireSAMCommand(t, conn, "SESSION CREATE STYLE=RAW")
		require.Contains(t, command, "DESTINATION=private-destination")
		require.NotContains(t, command, "DESTINATION=TRANSIENT")
		writeSAMReply(t, conn, "SESSION STATUS RESULT=OK DESTINATION=private-destination")
	})
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:    listener.Addr().String(),
		SAMUDPAddress: udp.LocalAddr().String(),
		Style:         DatagramStyleRaw,
		SessionID:     "anonbird-private-test",
		PrivateKey:    "private-destination",
	})
	require.NoError(t, err)
	require.NoError(t, session.Close())
}

func TestDatagramSessionValidation(t *testing.T) {
	session := &DatagramSession{id: "anonbird-test", style: DatagramStyleRaw}
	ctx := context.Background()

	err := session.Send(ctx, "peer.b32.i2p", make([]byte, MaxRawDatagramLen+1), SendOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "max")

	err = session.Send(ctx, "peer.b32.i2p", []byte("x"), SendOptions{Protocol: 17})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved")

	_, err = NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:    "127.0.0.1:1",
		SAMUDPAddress: "127.0.0.1:1",
		Style:         DatagramStyleRaw,
		SessionID:     "anonbird-test",
		InboundLength: 8,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tunnel length")

	_, err = NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:       "127.0.0.1:1",
		SAMUDPAddress:    "127.0.0.1:1",
		Style:            DatagramStyleRaw,
		SessionID:        "anonbird-test",
		OutboundQuantity: 17,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tunnel quantity")

	_, err = NewDatagramSession(ctx, DatagramConfig{
		SAMAddress:    "127.0.0.1:1",
		SAMUDPAddress: "127.0.0.1:1",
		Style:         "DATAGRAM3",
		SessionID:     "anonbird-test",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestDatagramReceiveRejectsMalformedSize(t *testing.T) {
	session := &DatagramSession{
		id:     "anonbird-test",
		style:  DatagramStyleRaw,
		reader: bufio.NewReader(strings.NewReader("RAW RECEIVED SIZE=not-a-number\n")),
	}

	_, err := session.Receive(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "SIZE")
}
