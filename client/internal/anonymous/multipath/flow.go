package multipath

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"net/netip"
)

const (
	ProtocolICMPv4 = 1
	ProtocolTCP    = 6
	ProtocolUDP    = 17
	ProtocolICMPv6 = 58
)

var (
	ErrShortPacket      = errors.New("packet is too short")
	ErrUnsupportedIP    = errors.New("unsupported IP version")
	ErrNoHealthyChannel = errors.New("no healthy multipath channels")
)

// FlowKey identifies an inner overlay flow before WireGuard encryption.
// It is direction-neutral so both sides of a TCP/UDP flow map to the same channel.
type FlowKey struct {
	Source      netip.Addr
	Destination netip.Addr
	Protocol    uint8
	SourcePort  uint16
	DestPort    uint16
	Fragmented  bool
}

// PacketInfo describes an overlay packet before WireGuard encryption.
type PacketInfo struct {
	Flow        FlowKey
	Source      netip.Addr
	Destination netip.Addr
}

// Channel represents one anonymous transport channel that can carry a flow.
type Channel struct {
	ID      string
	Healthy bool
}

// ClassifyPacket parses an IPv4 or IPv6 packet and returns a stable flow key.
func ClassifyPacket(packet []byte) (FlowKey, error) {
	info, err := ClassifyPacketInfo(packet)
	if err != nil {
		return FlowKey{}, err
	}
	return info.Flow, nil
}

// ClassifyPacketInfo parses an IPv4 or IPv6 packet and returns its original
// endpoints plus a direction-neutral flow key.
func ClassifyPacketInfo(packet []byte) (PacketInfo, error) {
	if len(packet) == 0 {
		return PacketInfo{}, ErrShortPacket
	}

	switch packet[0] >> 4 {
	case 4:
		return classifyIPv4(packet)
	case 6:
		return classifyIPv6(packet)
	default:
		return PacketInfo{}, ErrUnsupportedIP
	}
}

// SelectChannel picks a healthy channel with rendezvous hashing.
func SelectChannel(key FlowKey, channels []Channel) (Channel, error) {
	var (
		selected Channel
		best     uint64
		found    bool
	)

	for _, channel := range channels {
		if !channel.Healthy || channel.ID == "" {
			continue
		}
		score := key.score(channel.ID)
		if !found || score > best {
			best = score
			selected = channel
			found = true
		}
	}
	if !found {
		return Channel{}, ErrNoHealthyChannel
	}
	return selected, nil
}

func classifyIPv4(packet []byte) (PacketInfo, error) {
	if len(packet) < 20 {
		return PacketInfo{}, ErrShortPacket
	}
	headerLen := int(packet[0]&0x0f) * 4
	if headerLen < 20 || len(packet) < headerLen {
		return PacketInfo{}, ErrShortPacket
	}
	totalLen := int(binary.BigEndian.Uint16(packet[2:4]))
	if totalLen == 0 || totalLen > len(packet) || totalLen < headerLen {
		return PacketInfo{}, ErrShortPacket
	}
	if totalLen < len(packet) {
		packet = packet[:totalLen]
	}

	src, ok := netip.AddrFromSlice(packet[12:16])
	if !ok {
		return PacketInfo{}, fmt.Errorf("parse IPv4 source address")
	}
	dst, ok := netip.AddrFromSlice(packet[16:20])
	if !ok {
		return PacketInfo{}, fmt.Errorf("parse IPv4 destination address")
	}
	src = src.Unmap()
	dst = dst.Unmap()

	proto := packet[9]
	frag := binary.BigEndian.Uint16(packet[6:8])
	fragmented := frag&0x3fff != 0
	return PacketInfo{
		Flow:        canonicalFlow(src, dst, proto, packet[headerLen:], fragmented),
		Source:      src,
		Destination: dst,
	}, nil
}

func classifyIPv6(packet []byte) (PacketInfo, error) {
	if len(packet) < 40 {
		return PacketInfo{}, ErrShortPacket
	}
	payloadLen := int(binary.BigEndian.Uint16(packet[4:6]))
	if payloadLen != 0 {
		packetLen := 40 + payloadLen
		if packetLen > len(packet) {
			return PacketInfo{}, ErrShortPacket
		}
		if packetLen < len(packet) {
			packet = packet[:packetLen]
		}
	}

	src, ok := netip.AddrFromSlice(packet[8:24])
	if !ok {
		return PacketInfo{}, fmt.Errorf("parse IPv6 source address")
	}
	dst, ok := netip.AddrFromSlice(packet[24:40])
	if !ok {
		return PacketInfo{}, fmt.Errorf("parse IPv6 destination address")
	}
	src = src.Unmap()
	dst = dst.Unmap()

	nextHeader := packet[6]
	offset := 40
	for {
		switch nextHeader {
		case 0, 43, 60:
			if len(packet) < offset+2 {
				return PacketInfo{}, ErrShortPacket
			}
			nextHeader = packet[offset]
			offset += (int(packet[offset+1]) + 1) * 8
			if len(packet) < offset {
				return PacketInfo{}, ErrShortPacket
			}
		case 44:
			if len(packet) < offset+8 {
				return PacketInfo{}, ErrShortPacket
			}
			nextHeader = packet[offset]
			return PacketInfo{
				Flow:        canonicalFlow(src, dst, nextHeader, nil, true),
				Source:      src,
				Destination: dst,
			}, nil
		case 51:
			if len(packet) < offset+2 {
				return PacketInfo{}, ErrShortPacket
			}
			nextHeader = packet[offset]
			offset += (int(packet[offset+1]) + 2) * 4
			if len(packet) < offset {
				return PacketInfo{}, ErrShortPacket
			}
		default:
			return PacketInfo{
				Flow:        canonicalFlow(src, dst, nextHeader, packet[offset:], false),
				Source:      src,
				Destination: dst,
			}, nil
		}
	}
}

func canonicalFlow(src, dst netip.Addr, proto uint8, payload []byte, fragmented bool) FlowKey {
	srcPort, dstPort := ports(proto, payload, fragmented)
	key := FlowKey{
		Source:      src.Unmap(),
		Destination: dst.Unmap(),
		Protocol:    proto,
		SourcePort:  srcPort,
		DestPort:    dstPort,
		Fragmented:  fragmented,
	}
	if compareEndpoint(key.Source, key.SourcePort, key.Destination, key.DestPort) > 0 {
		key.Source, key.Destination = key.Destination, key.Source
		key.SourcePort, key.DestPort = key.DestPort, key.SourcePort
	}
	return key
}

func ports(proto uint8, payload []byte, fragmented bool) (uint16, uint16) {
	if fragmented {
		return 0, 0
	}
	switch proto {
	case ProtocolTCP, ProtocolUDP:
		if len(payload) < 4 {
			return 0, 0
		}
		return binary.BigEndian.Uint16(payload[0:2]), binary.BigEndian.Uint16(payload[2:4])
	case ProtocolICMPv4, ProtocolICMPv6:
		if len(payload) < 6 {
			return 0, 0
		}
		icmpType := payload[0]
		if proto == ProtocolICMPv4 && (icmpType == 0 || icmpType == 8) {
			id := binary.BigEndian.Uint16(payload[4:6])
			return id, id
		}
		if proto == ProtocolICMPv6 && (icmpType == 128 || icmpType == 129) {
			id := binary.BigEndian.Uint16(payload[4:6])
			return id, id
		}
	}
	return 0, 0
}

func compareEndpoint(a netip.Addr, aPort uint16, b netip.Addr, bPort uint16) int {
	if cmp := a.Compare(b); cmp != 0 {
		return cmp
	}
	switch {
	case aPort < bPort:
		return -1
	case aPort > bPort:
		return 1
	default:
		return 0
	}
}

func (k FlowKey) score(channelID string) uint64 {
	h := fnv.New64a()
	writeAddr(h, k.Source)
	writeAddr(h, k.Destination)
	var scalar [9]byte
	scalar[0] = k.Protocol
	binary.BigEndian.PutUint16(scalar[1:3], k.SourcePort)
	binary.BigEndian.PutUint16(scalar[3:5], k.DestPort)
	if k.Fragmented {
		scalar[5] = 1
	}
	_, _ = h.Write(scalar[:])
	_, _ = h.Write([]byte(channelID))
	return h.Sum64()
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeAddr(w byteWriter, addr netip.Addr) {
	raw := addr.As16()
	_, _ = w.Write(raw[:])
}
