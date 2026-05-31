package peer

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
)

type sentI2PDatagram struct {
	destination string
	payload     []byte
}

type fakeI2PDatagramSession struct {
	sendCh chan sentI2PDatagram
	recvCh chan i2psam.Datagram

	closeOnce sync.Once
	closed    chan struct{}
}

func newFakeI2PDatagramSession() *fakeI2PDatagramSession {
	return &fakeI2PDatagramSession{
		sendCh: make(chan sentI2PDatagram, 8),
		recvCh: make(chan i2psam.Datagram, 8),
		closed: make(chan struct{}),
	}
}

func (s *fakeI2PDatagramSession) Send(ctx context.Context, destination string, payload []byte, _ i2psam.SendOptions) error {
	select {
	case <-s.closed:
		return net.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case s.sendCh <- sentI2PDatagram{destination: destination, payload: append([]byte(nil), payload...)}:
		return nil
	}
}

func (s *fakeI2PDatagramSession) Receive(ctx context.Context) (i2psam.Datagram, error) {
	select {
	case <-s.closed:
		return i2psam.Datagram{}, net.ErrClosed
	case <-ctx.Done():
		return i2psam.Datagram{}, ctx.Err()
	case datagram := <-s.recvCh:
		return datagram, nil
	}
}

func (s *fakeI2PDatagramSession) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
	})
	return nil
}

func TestI2PDatagramPeerConnSendsAndReceivesByDestination(t *testing.T) {
	session := newFakeI2PDatagramSession()
	var seenConfig i2psam.DatagramConfig
	registry := newI2PDatagramRegistry(func(_ context.Context, cfg i2psam.DatagramConfig) (i2pDatagramSession, error) {
		seenConfig = cfg
		return session, nil
	})

	conn, err := registry.OpenPeerConn(context.Background(), testI2PTransport(), "peer-a", "remote-destination-a")
	require.NoError(t, err)
	defer conn.Close()

	require.Equal(t, i2psam.DatagramStyleRepliable, seenConfig.Style)
	require.Equal(t, "local-private-destination", seenConfig.PrivateKey)
	require.Equal(t, uint8(2), seenConfig.InboundLength)
	require.Equal(t, uint8(2), seenConfig.OutboundLength)
	require.Equal(t, uint8(4), seenConfig.InboundQuantity)
	require.Equal(t, uint8(4), seenConfig.OutboundQuantity)

	n, err := conn.Write([]byte("wg-packet"))
	require.NoError(t, err)
	require.Equal(t, len("wg-packet"), n)

	select {
	case sent := <-session.sendCh:
		require.Equal(t, "remote-destination-a", sent.destination)
		require.Equal(t, []byte("wg-packet"), sent.payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for i2p datagram send")
	}

	session.recvCh <- i2psam.Datagram{Source: "unknown-destination", Payload: []byte("drop-me")}
	session.recvCh <- i2psam.Datagram{Source: "remote-destination-a", Payload: []byte("wg-reply")}

	buf := make([]byte, 32)
	n, err = conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("wg-reply"), buf[:n])
}

func TestI2PDatagramRegistrySharesOneSAMSession(t *testing.T) {
	session := newFakeI2PDatagramSession()
	var calls int
	registry := newI2PDatagramRegistry(func(_ context.Context, _ i2psam.DatagramConfig) (i2pDatagramSession, error) {
		calls++
		return session, nil
	})

	connA, err := registry.OpenPeerConn(context.Background(), testI2PTransport(), "peer-a", "remote-destination-a")
	require.NoError(t, err)
	connB, err := registry.OpenPeerConn(context.Background(), testI2PTransport(), "peer-b", "remote-destination-b")
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	session.recvCh <- i2psam.Datagram{Source: "remote-destination-b", Payload: []byte("to-b")}
	buf := make([]byte, 16)
	n, err := connB.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte("to-b"), buf[:n])

	require.NoError(t, connA.Close())
	select {
	case <-session.closed:
		t.Fatal("shared i2p session closed while another peer still uses it")
	default:
	}

	require.NoError(t, connB.Close())
	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for shared i2p session close")
	}
}

func TestI2PDatagramPeerConnDeadlineAndValidation(t *testing.T) {
	session := newFakeI2PDatagramSession()
	registry := newI2PDatagramRegistry(func(_ context.Context, _ i2psam.DatagramConfig) (i2pDatagramSession, error) {
		return session, nil
	})

	_, err := registry.OpenPeerConn(context.Background(), testI2PTransport(), "peer-a", "")
	require.Error(t, err)

	conn, err := registry.OpenPeerConn(context.Background(), testI2PTransport(), "peer-a", "remote-destination-a")
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(-time.Second)))
	_, err = conn.Read(make([]byte, 8))
	require.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded))
}

func testI2PTransport() anonymous.TransportConfig {
	return anonymous.NormalizeTransport(anonymous.TransportConfig{
		Type:                  anonymous.TransportI2PDatagram,
		RequireAnonymous:      true,
		I2PSAM:                "127.0.0.1:7656",
		I2PTunnelLength:       2,
		I2PTunnelQuantity:     4,
		I2PDestinationPublic:  "local-public-destination",
		I2PDestinationPrivate: "local-private-destination",
	})
}
