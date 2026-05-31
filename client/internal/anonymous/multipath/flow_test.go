package multipath

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyIPv4TCPIsDirectionNeutral(t *testing.T) {
	forward := ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolTCP, tcpUDPHeader(40222, 22), 0)
	reverse := ipv4Packet(t, "100.80.0.20", "100.80.0.10", ProtocolTCP, tcpUDPHeader(22, 40222), 0)

	forwardKey, err := ClassifyPacket(forward)
	require.NoError(t, err)
	reverseKey, err := ClassifyPacket(reverse)
	require.NoError(t, err)

	require.Equal(t, forwardKey, reverseKey)
	require.Equal(t, uint8(ProtocolTCP), forwardKey.Protocol)
	require.Equal(t, uint16(40222), forwardKey.SourcePort)
	require.Equal(t, uint16(22), forwardKey.DestPort)
}

func TestClassifyPacketInfoKeepsOriginalEndpoints(t *testing.T) {
	packet := ipv4Packet(t, "100.80.0.20", "100.80.0.10", ProtocolTCP, tcpUDPHeader(22, 40222), 0)

	info, err := ClassifyPacketInfo(packet)
	require.NoError(t, err)

	require.Equal(t, netip.MustParseAddr("100.80.0.20"), info.Source)
	require.Equal(t, netip.MustParseAddr("100.80.0.10"), info.Destination)
	require.Equal(t, netip.MustParseAddr("100.80.0.10"), info.Flow.Source)
	require.Equal(t, netip.MustParseAddr("100.80.0.20"), info.Flow.Destination)
}

func TestClassifyIPv6UDPWithExtensionHeader(t *testing.T) {
	payload := append([]byte{ProtocolUDP, 0, 0, 0, 0, 0, 0, 0}, tcpUDPHeader(5353, 53000)...)
	packet := ipv6Packet(t, "fdf9:e2c9:1851::1", "fdf9:e2c9:1851::2", 60, payload)

	key, err := ClassifyPacket(packet)
	require.NoError(t, err)

	require.Equal(t, netip.MustParseAddr("fdf9:e2c9:1851::1"), key.Source)
	require.Equal(t, netip.MustParseAddr("fdf9:e2c9:1851::2"), key.Destination)
	require.Equal(t, uint8(ProtocolUDP), key.Protocol)
	require.Equal(t, uint16(5353), key.SourcePort)
	require.Equal(t, uint16(53000), key.DestPort)
}

func TestClassifyFragmentedPacketsIgnorePorts(t *testing.T) {
	firstFragment := ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolTCP, tcpUDPHeader(40222, 22), 0x2000)
	nextFragment := ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolTCP, nil, 0x2001)

	firstKey, err := ClassifyPacket(firstFragment)
	require.NoError(t, err)
	nextKey, err := ClassifyPacket(nextFragment)
	require.NoError(t, err)

	require.True(t, firstKey.Fragmented)
	require.Equal(t, firstKey, nextKey)
	require.Zero(t, firstKey.SourcePort)
	require.Zero(t, firstKey.DestPort)
}

func TestClassifyICMPEchoIsDirectionNeutral(t *testing.T) {
	request := ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolICMPv4, icmpEcho(8, 0x1234), 0)
	reply := ipv4Packet(t, "100.80.0.20", "100.80.0.10", ProtocolICMPv4, icmpEcho(0, 0x1234), 0)

	requestKey, err := ClassifyPacket(request)
	require.NoError(t, err)
	replyKey, err := ClassifyPacket(reply)
	require.NoError(t, err)

	require.Equal(t, requestKey, replyKey)
	require.Equal(t, uint8(ProtocolICMPv4), requestKey.Protocol)
	require.Equal(t, uint16(0x1234), requestKey.SourcePort)
	require.Equal(t, uint16(0x1234), requestKey.DestPort)
}

func TestSelectChannelIsFlowAffine(t *testing.T) {
	key, err := ClassifyPacket(ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolTCP, tcpUDPHeader(40222, 22), 0))
	require.NoError(t, err)
	channels := []Channel{
		{ID: "tor-stream-1", Healthy: true},
		{ID: "tor-stream-2", Healthy: true},
		{ID: "tor-stream-3", Healthy: true},
	}

	selected, err := SelectChannel(key, channels)
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		next, err := SelectChannel(key, channels)
		require.NoError(t, err)
		require.Equal(t, selected, next)
	}
}

func TestSelectChannelSkipsUnhealthyChannels(t *testing.T) {
	key, err := ClassifyPacket(ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolUDP, tcpUDPHeader(53000, 53), 0))
	require.NoError(t, err)

	selected, err := SelectChannel(key, []Channel{
		{ID: "tor-stream-1", Healthy: false},
		{ID: "tor-stream-2", Healthy: true},
	})
	require.NoError(t, err)
	require.Equal(t, "tor-stream-2", selected.ID)

	_, err = SelectChannel(key, []Channel{{ID: "tor-stream-1", Healthy: false}})
	require.ErrorIs(t, err, ErrNoHealthyChannel)
}

func TestClassifyRejectsShortAndUnknownPackets(t *testing.T) {
	_, err := ClassifyPacket(nil)
	require.ErrorIs(t, err, ErrShortPacket)

	_, err = ClassifyPacket([]byte{0xf0})
	require.ErrorIs(t, err, ErrUnsupportedIP)
}

func TestClassifyRejectsMalformedLengths(t *testing.T) {
	ipv4 := ipv4Packet(t, "100.80.0.10", "100.80.0.20", ProtocolTCP, tcpUDPHeader(40222, 22), 0)
	binary.BigEndian.PutUint16(ipv4[2:4], uint16(len(ipv4)+1))
	_, err := ClassifyPacket(ipv4)
	require.ErrorIs(t, err, ErrShortPacket)

	ipv6 := ipv6Packet(t, "fdf9:e2c9:1851::1", "fdf9:e2c9:1851::2", ProtocolUDP, tcpUDPHeader(5353, 53000))
	binary.BigEndian.PutUint16(ipv6[4:6], uint16(len(ipv6)-39))
	_, err = ClassifyPacket(ipv6)
	require.ErrorIs(t, err, ErrShortPacket)
}

func tcpUDPHeader(src, dst uint16) []byte {
	header := make([]byte, 4)
	binary.BigEndian.PutUint16(header[0:2], src)
	binary.BigEndian.PutUint16(header[2:4], dst)
	return header
}

func icmpEcho(typ uint8, id uint16) []byte {
	payload := make([]byte, 8)
	payload[0] = typ
	binary.BigEndian.PutUint16(payload[4:6], id)
	return payload
}

func ipv4Packet(t *testing.T, src, dst string, proto uint8, payload []byte, frag uint16) []byte {
	t.Helper()
	packet := make([]byte, 20+len(payload))
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	binary.BigEndian.PutUint16(packet[6:8], frag)
	packet[8] = 64
	packet[9] = proto
	srcAddr := netip.MustParseAddr(src).As4()
	dstAddr := netip.MustParseAddr(dst).As4()
	copy(packet[12:16], srcAddr[:])
	copy(packet[16:20], dstAddr[:])
	copy(packet[20:], payload)
	return packet
}

func ipv6Packet(t *testing.T, src, dst string, nextHeader uint8, payload []byte) []byte {
	t.Helper()
	packet := make([]byte, 40+len(payload))
	packet[0] = 0x60
	binary.BigEndian.PutUint16(packet[4:6], uint16(len(payload)))
	packet[6] = nextHeader
	packet[7] = 64
	srcAddr := netip.MustParseAddr(src).As16()
	dstAddr := netip.MustParseAddr(dst).As16()
	copy(packet[8:24], srcAddr[:])
	copy(packet[24:40], dstAddr[:])
	copy(packet[40:], payload)
	return packet
}
