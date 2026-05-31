package peer

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/anonymous/multipath"
)

func TestRelayMultipathConnRoutesDataPacketsByFlowHint(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(4)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packetA, channelA := relayMultipathPacketForChannel(t, channels, func(id uint32) bool { return true })
	packetB, channelB := relayMultipathPacketForChannel(t, channels, func(id uint32) bool { return id != channelA })
	require.NotEqual(t, channelA, channelB)

	conn.ObservePacket(packetA, true)
	_, err := conn.Write(wireGuardDataPacket("flow-a"))
	require.NoError(t, err)
	require.Equal(t, wireGuardDataPacket("flow-a"), <-fakes[channelA].writes)

	conn.ObservePacket(packetB, true)
	_, err = conn.Write(wireGuardDataPacket("flow-b"))
	require.NoError(t, err)
	require.Equal(t, wireGuardDataPacket("flow-b"), <-fakes[channelB].writes)
}

func TestRelayMultipathConnDoesNotConsumeHintsForWireGuardHandshake(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(4)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packet, selectedChannel := relayMultipathPacketForChannel(t, channels, func(id uint32) bool { return id != 0 })

	conn.ObservePacket(packet, true)
	_, err := conn.Write([]byte{1, 0, 0, 0, 'h', 's'})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 0, 0, 0, 'h', 's'}, <-fakes[0].writes)

	_, err = conn.Write(wireGuardDataPacket("after-handshake"))
	require.NoError(t, err)
	require.Equal(t, wireGuardDataPacket("after-handshake"), <-fakes[selectedChannel].writes)
}

func TestRelayMultipathConnIgnoresPacketsOutsidePeerAllowedIPs(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	conn.ObservePacket(relayMultipathIPv4Packet(t, "100.80.0.10", "100.80.0.30", 40000), true)
	_, err := conn.Write(wireGuardDataPacket("default"))
	require.NoError(t, err)
	require.Equal(t, wireGuardDataPacket("default"), <-fakes[0].writes)
}

func TestRelayMultipathConnReadsFromAnyChannel(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	fakes[1].reads <- []byte("from-channel-1")
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "from-channel-1", string(buf[:n]))
}

func TestRelayMultipathConnSkipsUnhealthyChannelAfterWriteError(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(3)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packet, failedChannel := relayMultipathPacketForChannel(t, channels, func(id uint32) bool { return id == 1 })
	fakes[failedChannel].writeErr = errors.New("channel failed")

	conn.ObservePacket(packet, true)
	_, err := conn.Write(wireGuardDataPacket("first"))
	require.Error(t, err)

	conn.ObservePacket(packet, true)
	_, err = conn.Write(wireGuardDataPacket("second"))
	require.NoError(t, err)
	select {
	case got := <-fakes[failedChannel].writes:
		t.Fatalf("unhealthy channel received write %q", string(got))
	default:
	}
	require.Equal(t, wireGuardDataPacket("second"), <-fakes[0].writes)
}

func TestRelayMultipathConnKeepsRunningAfterSecondaryReadError(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	fakes[1].readErrs <- errors.New("secondary failed")
	require.Eventually(t, func() bool {
		return fakes[1].isClosed()
	}, time.Second, 10*time.Millisecond)

	fakes[0].reads <- []byte("primary-still-works")
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "primary-still-works", string(buf[:n]))
}

type relayMultipathFakeConn struct {
	writes   chan []byte
	reads    chan []byte
	readErrs chan error
	closed   chan struct{}
	writeErr error
}

func newTestRelayMultipathChannels(count int) ([]relayMultipathChannel, map[uint32]*relayMultipathFakeConn) {
	channels := make([]relayMultipathChannel, 0, count)
	fakes := make(map[uint32]*relayMultipathFakeConn, count)
	for i := 0; i < count; i++ {
		id := uint32(i)
		fake := &relayMultipathFakeConn{
			writes:   make(chan []byte, 8),
			reads:    make(chan []byte, 8),
			readErrs: make(chan error, 1),
			closed:   make(chan struct{}),
		}
		channels = append(channels, relayMultipathChannel{id: id, conn: fake})
		fakes[id] = fake
	}
	return channels, fakes
}

func (c *relayMultipathFakeConn) Read(b []byte) (int, error) {
	select {
	case packet := <-c.reads:
		return copy(b, packet), nil
	case err := <-c.readErrs:
		return 0, err
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *relayMultipathFakeConn) Write(b []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	packet := append([]byte(nil), b...)
	select {
	case c.writes <- packet:
		return len(b), nil
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *relayMultipathFakeConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *relayMultipathFakeConn) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

func (c *relayMultipathFakeConn) LocalAddr() net.Addr              { return relayMultipathTestAddr("local") }
func (c *relayMultipathFakeConn) RemoteAddr() net.Addr             { return relayMultipathTestAddr("remote") }
func (c *relayMultipathFakeConn) SetDeadline(time.Time) error      { return nil }
func (c *relayMultipathFakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *relayMultipathFakeConn) SetWriteDeadline(time.Time) error { return nil }
func (c relayMultipathTestAddr) Network() string                   { return string(c) }
func (c relayMultipathTestAddr) String() string                    { return string(c) }

type relayMultipathTestAddr string

func relayMultipathPacketForChannel(t *testing.T, channels []relayMultipathChannel, accept func(uint32) bool) ([]byte, uint32) {
	t.Helper()

	selectorChannels := make([]multipath.Channel, 0, len(channels))
	selectorIDToChannel := make(map[string]uint32, len(channels))
	for _, channel := range channels {
		selectorID := strconv.FormatUint(uint64(channel.id), 10)
		selectorChannels = append(selectorChannels, multipath.Channel{ID: selectorID, Healthy: true})
		selectorIDToChannel[selectorID] = channel.id
	}

	for port := uint16(30000); port < 65000; port++ {
		packet := relayMultipathIPv4Packet(t, "100.80.0.10", "100.80.0.20", port)
		info, err := multipath.ClassifyPacketInfo(packet)
		require.NoError(t, err)
		selected, err := multipath.SelectChannel(info.Flow, selectorChannels)
		require.NoError(t, err)
		channelID := selectorIDToChannel[selected.ID]
		if accept(channelID) {
			return packet, channelID
		}
	}
	t.Fatal("failed to find packet for requested relay multipath channel")
	return nil, 0
}

func relayMultipathIPv4Packet(t *testing.T, src, dst string, srcPort uint16) []byte {
	t.Helper()
	packet := make([]byte, 24)
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	packet[8] = 64
	packet[9] = multipath.ProtocolTCP
	srcAddr := netip.MustParseAddr(src).As4()
	dstAddr := netip.MustParseAddr(dst).As4()
	copy(packet[12:16], srcAddr[:])
	copy(packet[16:20], dstAddr[:])
	binary.BigEndian.PutUint16(packet[20:22], srcPort)
	binary.BigEndian.PutUint16(packet[22:24], 443)
	return packet
}

func wireGuardDataPacket(payload string) []byte {
	return append([]byte{4, 0, 0, 0}, payload...)
}
