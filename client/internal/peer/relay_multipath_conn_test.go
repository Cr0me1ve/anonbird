package peer

import (
	"context"
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

func TestRelayMultipathConnBalancesNewFlowHintsAcrossLeastLoadedChannels(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(4)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	for i := 0; i < len(channels); i++ {
		conn.ObservePacket(relayMultipathIPv4Packet(t, "100.80.0.10", "100.80.0.20", uint16(40000+i)), true)
		_, err := conn.Write(wireGuardDataPacket("flow-" + strconv.Itoa(i)))
		require.NoError(t, err)
	}

	used := make(map[uint32]bool)
	for channelID, fake := range fakes {
		select {
		case <-fake.writes:
			used[channelID] = true
		default:
		}
	}
	require.Len(t, used, len(channels))
	for _, count := range conn.flowChannelCounts {
		require.Equal(t, 1, count)
	}
}

func TestRelayMultipathConnKeepsExistingFlowHintSticky(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(4)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packet := relayMultipathIPv4Packet(t, "100.80.0.10", "100.80.0.20", 40000)
	first := wireGuardDataPacket("first")
	conn.ObservePacket(packet, true)
	_, err := conn.Write(first)
	require.NoError(t, err)

	var assignedChannel uint32
	assignedCount := 0
	for channelID, fake := range fakes {
		select {
		case got := <-fake.writes:
			require.Equal(t, first, got)
			assignedChannel = channelID
			assignedCount++
		default:
		}
	}
	require.Equal(t, 1, assignedCount)

	second := wireGuardDataPacket("second")
	conn.ObservePacket(packet, true)
	_, err = conn.Write(second)
	require.NoError(t, err)
	require.Equal(t, second, <-fakes[assignedChannel].writes)
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

func TestRelayMultipathConnPacketBurstStripesSingleFlow(t *testing.T) {
	t.Setenv(envAnonRelayMultipathStrategy, relayMultipathStrategyPacketBurst)
	t.Setenv(envAnonRelayMultipathPacketBurstSize, "3")
	channels, fakes := newTestRelayMultipathChannels(4)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()
	require.Equal(t, uint64(3), conn.packetBurstSize)

	packet := wireGuardDataPacket("bulk")
	for i := 0; i < int(conn.packetBurstSize)*2; i++ {
		_, err := conn.Write(packet)
		require.NoError(t, err)
	}

	for i := 0; i < int(conn.packetBurstSize); i++ {
		require.Equal(t, packet, <-fakes[0].writes)
	}
	for i := 0; i < int(conn.packetBurstSize); i++ {
		require.Equal(t, packet, <-fakes[1].writes)
	}
	select {
	case got := <-fakes[2].writes:
		t.Fatalf("unexpected write on channel 2 before the next burst: %q", string(got))
	default:
	}
}

func TestRelayMultipathConnPacketBurstSkipsUnhealthyChannel(t *testing.T) {
	t.Setenv(envAnonRelayMultipathStrategy, relayMultipathStrategyPacketBurst)
	t.Setenv(envAnonRelayMultipathPacketBurstSize, "3")
	channels, fakes := newTestRelayMultipathChannels(3)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packet := wireGuardDataPacket("bulk")
	for i := 0; i < int(conn.packetBurstSize); i++ {
		_, err := conn.Write(packet)
		require.NoError(t, err)
		require.Equal(t, packet, <-fakes[0].writes)
	}

	fakes[1].writeErr = errors.New("channel failed")
	_, err := conn.Write(packet)
	require.NoError(t, err)
	require.Equal(t, packet, <-fakes[0].writes)

	for i := 0; i < int(conn.packetBurstSize)-1; i++ {
		_, err = conn.Write(packet)
		require.NoError(t, err)
		require.Equal(t, packet, <-fakes[2].writes)
	}
	_, err = conn.Write(packet)
	require.NoError(t, err)
	require.Equal(t, packet, <-fakes[0].writes)
	select {
	case got := <-fakes[1].writes:
		t.Fatalf("unhealthy channel received write %q", string(got))
	default:
	}
}

func TestRelayMultipathConnPacketBurstSkipsSlowChannelDuringCooldown(t *testing.T) {
	t.Setenv(envAnonRelayMultipathStrategy, relayMultipathStrategyPacketBurst)
	t.Setenv(envAnonRelayMultipathPacketBurstSize, "1")
	t.Setenv(envAnonRelayMultipathSlowWriteMS, "1")
	t.Setenv(envAnonRelayMultipathCooldownMS, "1000")
	channels, fakes := newTestRelayMultipathChannels(2)
	fakes[0].writeDelay = 10 * time.Millisecond
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	first := wireGuardDataPacket("slow-first")
	_, err := conn.Write(first)
	require.NoError(t, err)
	require.Equal(t, first, <-fakes[0].writes)
	require.True(t, conn.channelCooling(0, time.Now()))

	second := wireGuardDataPacket("after-slow")
	_, err = conn.Write(second)
	require.NoError(t, err)
	require.Equal(t, second, <-fakes[1].writes)
	select {
	case got := <-fakes[0].writes:
		t.Fatalf("cooling channel received write %q", string(got))
	default:
	}
}

func TestRelayMultipathConnPacketBurstSkipsChannelOverInflightLimit(t *testing.T) {
	t.Setenv(envAnonRelayMultipathStrategy, relayMultipathStrategyPacketBurst)
	t.Setenv(envAnonRelayMultipathPacketBurstSize, "4")
	t.Setenv(envAnonRelayMultipathMaxInflight, "32")
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	conn.channelStats[0].activeWriteBytes.Store(conn.maxInflight)
	packet := wireGuardDataPacket("bulk")
	_, err := conn.Write(packet)
	require.NoError(t, err)
	require.Equal(t, packet, <-fakes[1].writes)
	select {
	case got := <-fakes[0].writes:
		t.Fatalf("over-limit channel received write %q", string(got))
	default:
	}
}

func TestRelayMultipathConnFlowAffineAvoidsSilentPreferredChannel(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	conn.recordChannelRead(0, 128)
	conn.channelStats[1].selectedWrites.Store(relayMultipathSilentWriteThreshold)
	conn.enqueueHint(1)

	packet := wireGuardDataPacket("bulk")
	_, err := conn.Write(packet)
	require.NoError(t, err)
	require.Equal(t, packet, <-fakes[0].writes)
	select {
	case got := <-fakes[1].writes:
		t.Fatalf("silent preferred channel received write %q", string(got))
	default:
	}
}

func TestRelayMultipathConnBatchesWireGuardDataPackets(t *testing.T) {
	t.Setenv(envAnonRelayMultipathBatch, "true")
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()
	require.True(t, conn.batchingEnabled)

	packets := [][]byte{
		wireGuardDataPacket("batch-1"),
		wireGuardDataPacket("batch-2"),
		wireGuardDataPacket("batch-3"),
	}
	for _, packet := range packets {
		n, err := conn.Write(packet)
		require.NoError(t, err)
		require.Equal(t, len(packet), n)
	}

	var written []byte
	require.Eventually(t, func() bool {
		select {
		case written = <-fakes[0].writes:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	decoded, ok := decodeRelayPacketBatch(written)
	require.True(t, ok)
	require.Equal(t, packets, decoded)
}

func TestRelayMultipathConnBatcherReleasesPendingBytesAfterFlush(t *testing.T) {
	t.Setenv(envAnonRelayMultipathBatch, "true")
	channels, fakes := newTestRelayMultipathChannels(1)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packet := wireGuardDataPacket("pending")
	_, err := conn.Write(packet)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		select {
		case <-fakes[0].writes:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return conn.channelInFlightBytes(0) == 0
	}, time.Second, 10*time.Millisecond)
}

func TestRelayMultipathConnReadsBatchedWireGuardPackets(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	packets := [][]byte{
		wireGuardDataPacket("read-batch-1"),
		wireGuardDataPacket("read-batch-2"),
	}
	fakes[1].reads <- encodeRelayPacketBatch(packets)

	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, packets[0], buf[:n])

	n, err = conn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, packets[1], buf[:n])
}

func TestRelayMultipathConnSkipsCongestedBatchChannel(t *testing.T) {
	t.Setenv(envAnonRelayMultipathStrategy, relayMultipathStrategyPacketBurst)
	t.Setenv(envAnonRelayMultipathPacketBurstSize, "1")
	t.Setenv(envAnonRelayMultipathBatch, "true")
	t.Setenv(envAnonRelayMultipathCooldownMS, "1000")
	channels, fakes := newTestRelayMultipathChannels(2)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	congestedBatcher := &relayMultipathBatcher{
		parent:  conn,
		conn:    fakes[0],
		channel: 0,
		queue:   make(chan []byte, 1),
		done:    make(chan struct{}),
	}
	congestedBatcher.queue <- wireGuardDataPacket("queued")
	conn.batchMu.Lock()
	conn.batchers[0] = congestedBatcher
	conn.batchMu.Unlock()

	packet := wireGuardDataPacket("bulk")
	n, err := conn.Write(packet)
	require.NoError(t, err)
	require.Equal(t, len(packet), n)
	require.True(t, conn.channelCooling(0, time.Now()))

	var written []byte
	require.Eventually(t, func() bool {
		select {
		case written = <-fakes[1].writes:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, packet, written)
	select {
	case got := <-fakes[0].writes:
		t.Fatalf("congested channel received write %q", string(got))
	default:
	}
}

func TestRelayMultipathConnAdaptiveBatchLimits(t *testing.T) {
	t.Setenv(envAnonRelayMultipathMaxInflight, "1024")
	channels, _ := newTestRelayMultipathChannels(1)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	maxPackets, maxBytes := conn.batchLimits(0)
	require.Equal(t, relayMultipathBatchMaxPackets, maxPackets)
	require.Equal(t, relayMultipathBatchMaxBytes, maxBytes)

	conn.channelStats[0].pendingWriteBytes.Store(600)
	maxPackets, maxBytes = conn.batchLimits(0)
	require.Equal(t, 2, maxPackets)
	require.Equal(t, 4096, maxBytes)

	conn.channelStats[0].lastReadUnixNano.Store(time.Now().Add(-conn.readIdlePenalty).UnixNano())
	maxPackets, maxBytes = conn.batchLimits(0)
	require.Equal(t, 1, maxPackets)
	require.Equal(t, 2048, maxBytes)
}

func TestRelayMultipathTelemetrySnapshotIncludesChannelStats(t *testing.T) {
	t.Setenv(envAnonRelayMultipathBatch, "true")
	channels, _ := newTestRelayMultipathChannels(1)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	conn.recordChannelSelected(0)
	conn.recordChannelRead(0, 512)
	conn.recordChannelWrite(0, 10*time.Millisecond, 256, nil)
	conn.markChannelCongested(0)
	require.True(t, conn.tryReserveChannelPendingBytes(0, 128))
	conn.addChannelActiveWriteBytes(0, 64)

	snapshot := conn.telemetrySnapshot(time.Now())
	require.Len(t, snapshot, 1)
	require.Equal(t, uint32(0), snapshot[0].id)
	require.True(t, snapshot[0].healthy)
	require.Equal(t, int64(128), snapshot[0].pendingBytes)
	require.Equal(t, int64(64), snapshot[0].activeBytes)
	require.Equal(t, uint64(1), snapshot[0].selectedWrites)
	require.Equal(t, uint64(1), snapshot[0].readFrames)
	require.Equal(t, uint64(512), snapshot[0].readBytes)
	require.Equal(t, uint64(256), snapshot[0].writtenBytes)
	require.Equal(t, uint64(1), snapshot[0].congestions)
	require.Positive(t, snapshot[0].writeEWMA)
}

func TestRelayMultipathPacketBurstSizeFallsBackAndCaps(t *testing.T) {
	t.Setenv(envAnonRelayMultipathPacketBurstSize, "bad")
	require.Equal(t, uint64(relayMultipathDefaultPacketBurstSize), relayMultipathPacketBurstSize())

	t.Setenv(envAnonRelayMultipathPacketBurstSize, "0")
	require.Equal(t, uint64(relayMultipathDefaultPacketBurstSize), relayMultipathPacketBurstSize())

	t.Setenv(envAnonRelayMultipathPacketBurstSize, strconv.Itoa(relayMultipathMaxPacketBurstSize+1))
	require.Equal(t, uint64(relayMultipathMaxPacketBurstSize), relayMultipathPacketBurstSize())
}

func TestRelayMultipathPacingAndInflightCanBeDisabled(t *testing.T) {
	t.Setenv(envAnonRelayMultipathPacingMS, "0")
	require.Zero(t, relayMultipathDurationFromMSAllowZero(envAnonRelayMultipathPacingMS, relayMultipathDefaultPacingDelay))

	t.Setenv(envAnonRelayMultipathMaxInflight, "0")
	require.Zero(t, relayMultipathInt64FromEnvAllowZero(envAnonRelayMultipathMaxInflight, relayMultipathDefaultMaxInflight))
}

func TestRelayMultipathConnReopensUnhealthyChannel(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(3)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	reopened := newRelayMultipathFakeConn()
	reopenedCh := make(chan uint32, 4)
	conn.setChannelReopener(func(_ context.Context, channelID uint32) (net.Conn, error) {
		reopenedCh <- channelID
		return reopened, nil
	})

	packet, failedChannel := relayMultipathPacketForChannel(t, channels, func(id uint32) bool { return id == 1 })
	fakes[failedChannel].writeErr = errors.New("channel failed")

	conn.ObservePacket(packet, true)
	_, err := conn.Write(wireGuardDataPacket("first"))
	require.NoError(t, err)
	require.Equal(t, wireGuardDataPacket("first"), <-fakes[0].writes)
	require.Equal(t, failedChannel, <-reopenedCh)
	require.Eventually(t, func() bool {
		return relayMultipathChannelHealthy(conn, failedChannel)
	}, time.Second, 10*time.Millisecond)

	conn.ObservePacket(packet, true)
	_, err = conn.Write(wireGuardDataPacket("after-reopen"))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		select {
		case got := <-reopened.writes:
			require.Equal(t, wireGuardDataPacket("after-reopen"), got)
			return true
		case got := <-fakes[0].writes:
			t.Fatalf("fallback channel received write after reopen: %q", string(got))
		case got := <-fakes[2].writes:
			t.Fatalf("unhinted channel received write after reopen: %q", string(got))
		default:
			return false
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

func TestRelayMultipathConnDoesNotWaitForChannelReopen(t *testing.T) {
	channels, fakes := newTestRelayMultipathChannels(3)
	conn := newRelayMultipathConn(channels, []netip.Prefix{netip.MustParsePrefix("100.80.0.20/32")}, nil)
	defer conn.Close()

	reopenStarted := make(chan struct{}, 1)
	releaseReopen := make(chan struct{})
	conn.setChannelReopener(func(ctx context.Context, _ uint32) (net.Conn, error) {
		select {
		case reopenStarted <- struct{}{}:
		default:
		}
		select {
		case <-releaseReopen:
			return newRelayMultipathFakeConn(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	defer close(releaseReopen)

	packet, failedChannel := relayMultipathPacketForChannel(t, channels, func(id uint32) bool { return id == 1 })
	fakes[failedChannel].writeErr = errors.New("channel failed")

	conn.ObservePacket(packet, true)
	start := time.Now()
	_, err := conn.Write(wireGuardDataPacket("first"))
	require.NoError(t, err)
	require.Less(t, time.Since(start), 100*time.Millisecond)
	require.Equal(t, wireGuardDataPacket("first"), <-fakes[0].writes)
	require.Eventually(t, func() bool {
		select {
		case <-reopenStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
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
	require.NoError(t, err)
	require.Equal(t, wireGuardDataPacket("first"), <-fakes[0].writes)

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
	writes     chan []byte
	reads      chan []byte
	readErrs   chan error
	closed     chan struct{}
	writeErr   error
	writeDelay time.Duration
}

func newTestRelayMultipathChannels(count int) ([]relayMultipathChannel, map[uint32]*relayMultipathFakeConn) {
	channels := make([]relayMultipathChannel, 0, count)
	fakes := make(map[uint32]*relayMultipathFakeConn, count)
	for i := 0; i < count; i++ {
		id := uint32(i)
		fake := newRelayMultipathFakeConn()
		channels = append(channels, relayMultipathChannel{id: id, conn: fake})
		fakes[id] = fake
	}
	return channels, fakes
}

func newRelayMultipathFakeConn() *relayMultipathFakeConn {
	return &relayMultipathFakeConn{
		writes:   make(chan []byte, 8),
		reads:    make(chan []byte, 8),
		readErrs: make(chan error, 1),
		closed:   make(chan struct{}),
	}
}

func relayMultipathChannelHealthy(conn *relayMultipathConn, channelID uint32) bool {
	conn.selectorMu.RLock()
	defer conn.selectorMu.RUnlock()
	return conn.channelHealthyLocked(channelID)
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
	if c.writeDelay > 0 {
		select {
		case <-time.After(c.writeDelay):
		case <-c.closed:
			return 0, net.ErrClosed
		}
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
