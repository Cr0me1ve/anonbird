package peer

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netbirdio/netbird/client/internal/anonymous/multipath"
	log "github.com/sirupsen/logrus"
)

const (
	relayMultipathHintQueueSize = 256
	relayMultipathReadQueueSize = 64
	relayMultipathReadBuffer    = 65535

	relayMultipathStrategyFlowAffine     = "flow-affine"
	relayMultipathStrategyPacketBurst    = "packet-burst"
	relayMultipathDefaultPacketBurstSize = 64
	relayMultipathMaxPacketBurstSize     = 4096
	relayMultipathDefaultSlowWrite       = 250 * time.Millisecond
	relayMultipathDefaultCooldown        = 3 * time.Second
	relayMultipathDefaultReadIdlePenalty = 20 * time.Second
	relayMultipathDefaultPacingDelay     = 2 * time.Millisecond
	relayMultipathDefaultMaxInflight     = 128 * 1024

	envAnonRelayMultipathStrategy        = "NB_ANON_RELAY_MULTIPATH_STRATEGY"
	envAnonRelayMultipathPacketBurstSize = "NB_ANON_RELAY_MULTIPATH_PACKET_BURST_SIZE"
	envAnonRelayMultipathBatch           = "NB_ANON_RELAY_MULTIPATH_BATCH"
	envAnonRelayMultipathScoring         = "NB_ANON_RELAY_MULTIPATH_CHANNEL_SCORING"
	envAnonRelayMultipathSlowWriteMS     = "NB_ANON_RELAY_MULTIPATH_SLOW_WRITE_MS"
	envAnonRelayMultipathCooldownMS      = "NB_ANON_RELAY_MULTIPATH_CHANNEL_COOLDOWN_MS"
	envAnonRelayMultipathReadIdleMS      = "NB_ANON_RELAY_MULTIPATH_READ_IDLE_MS"
	envAnonRelayMultipathPacingMS        = "NB_ANON_RELAY_MULTIPATH_PACING_MS"
	envAnonRelayMultipathMaxInflight     = "NB_ANON_RELAY_MULTIPATH_CHANNEL_MAX_INFLIGHT_BYTES"
	envAnonRelayMultipathTelemetryMS     = "NB_ANON_RELAY_MULTIPATH_TELEMETRY_MS"

	relayMultipathReopenTimeout        = 30 * time.Second
	relayMultipathReopenInitialBackoff = time.Second
	relayMultipathReopenMaxBackoff     = 30 * time.Second

	relayMultipathBatchQueueSize    = 512
	relayMultipathBatchMaxPackets   = 6
	relayMultipathBatchMaxBytes     = 8192
	relayMultipathBatchFlushDelay   = 2 * time.Millisecond
	relayMultipathBatchHeaderSize   = 6
	relayMultipathBatchPacketHeader = 2
)

var relayMultipathBatchMagic = [4]byte{0xab, 0x1d, 0xba, 0x7c}

var errRelayMultipathChannelCongested = errors.New("relay multipath channel congested")

type relayMultipathChannel struct {
	id   uint32
	conn net.Conn
}

type relayMultipathChannelReopener func(ctx context.Context, channelID uint32) (net.Conn, error)

type relayMultipathRead struct {
	payload []byte
	err     error
}

type relayMultipathChannelStats struct {
	lastReadUnixNano  atomic.Int64
	lastWriteUnixNano atomic.Int64
	pendingWriteBytes atomic.Int64
	activeWriteBytes  atomic.Int64
	writeEWMAMicros   atomic.Int64
	slowUntilUnixNano atomic.Int64
	nextWriteUnixNano atomic.Int64
	writeFailures     atomic.Uint64
	selectedWrites    atomic.Uint64
	readFrames        atomic.Uint64
	readBytes         atomic.Uint64
	writtenBytes      atomic.Uint64
	congestions       atomic.Uint64
}

type relayMultipathWriteCandidate struct {
	id    uint32
	conn  net.Conn
	score int64
}

type relayMultipathTelemetryChannel struct {
	id             uint32
	healthy        bool
	pendingBytes   int64
	activeBytes    int64
	writeEWMA      time.Duration
	cooldown       time.Duration
	pacingWait     time.Duration
	readIdle       time.Duration
	writeIdle      time.Duration
	queueLen       int
	selectedWrites uint64
	readFrames     uint64
	readBytes      uint64
	writtenBytes   uint64
	writeFailures  uint64
	congestions    uint64
}

type relayMultipathConn struct {
	channels            map[uint32]net.Conn
	selectorChannels    []multipath.Channel
	selectorIDToChannel map[string]uint32
	channelStats        map[uint32]*relayMultipathChannelStats
	selectorMu          sync.RWMutex
	allowedIPs          []netip.Prefix
	primaryChannelID    uint32

	hintCh chan uint32
	readCh chan relayMultipathRead
	done   chan struct{}

	unregisterObserver func()
	observerMu         sync.Mutex
	closeOnce          sync.Once
	closed             atomic.Bool

	errMu   sync.Mutex
	readErr error

	writeStrategy   string
	packetBurstSize uint64
	stripeCounter   atomic.Uint64
	scoringEnabled  bool
	slowWrite       time.Duration
	channelCooldown time.Duration
	readIdlePenalty time.Duration
	pacingDelay     time.Duration
	maxInflight     int64
	telemetryEvery  time.Duration

	reopenMu  sync.Mutex
	reopener  relayMultipathChannelReopener
	reopening map[uint32]struct{}

	batchingEnabled bool
	batchMu         sync.Mutex
	batchers        map[uint32]*relayMultipathBatcher
	writeLocks      map[uint32]*sync.Mutex
}

func newRelayMultipathConn(channels []relayMultipathChannel, allowedIPs []netip.Prefix, unregisterObserver func()) *relayMultipathConn {
	c := &relayMultipathConn{
		channels:            make(map[uint32]net.Conn, len(channels)),
		selectorChannels:    make([]multipath.Channel, 0, len(channels)),
		selectorIDToChannel: make(map[string]uint32, len(channels)),
		channelStats:        make(map[uint32]*relayMultipathChannelStats, len(channels)),
		allowedIPs:          append([]netip.Prefix(nil), allowedIPs...),
		hintCh:              make(chan uint32, relayMultipathHintQueueSize),
		readCh:              make(chan relayMultipathRead, relayMultipathReadQueueSize),
		done:                make(chan struct{}),
		unregisterObserver:  unregisterObserver,
		writeStrategy:       relayMultipathWriteStrategy(),
		packetBurstSize:     relayMultipathPacketBurstSize(),
		scoringEnabled:      relayMultipathChannelScoringEnabled(),
		slowWrite:           relayMultipathDurationFromMS(envAnonRelayMultipathSlowWriteMS, relayMultipathDefaultSlowWrite),
		channelCooldown:     relayMultipathDurationFromMS(envAnonRelayMultipathCooldownMS, relayMultipathDefaultCooldown),
		readIdlePenalty:     relayMultipathDurationFromMS(envAnonRelayMultipathReadIdleMS, relayMultipathDefaultReadIdlePenalty),
		pacingDelay:         relayMultipathDurationFromMSAllowZero(envAnonRelayMultipathPacingMS, relayMultipathDefaultPacingDelay),
		maxInflight:         relayMultipathInt64FromEnvAllowZero(envAnonRelayMultipathMaxInflight, relayMultipathDefaultMaxInflight),
		telemetryEvery:      relayMultipathDurationFromMSAllowZero(envAnonRelayMultipathTelemetryMS, 0),
		reopening:           make(map[uint32]struct{}),
		batchingEnabled:     relayMultipathBatchingEnabled(),
		batchers:            make(map[uint32]*relayMultipathBatcher),
		writeLocks:          make(map[uint32]*sync.Mutex),
	}
	now := time.Now()
	for i, channel := range channels {
		if i == 0 {
			c.primaryChannelID = channel.id
		}
		c.channels[channel.id] = channel.conn
		c.channelStats[channel.id] = newRelayMultipathChannelStats(now)
		selectorID := strconv.FormatUint(uint64(channel.id), 10)
		c.selectorChannels = append(c.selectorChannels, multipath.Channel{ID: selectorID, Healthy: true})
		c.selectorIDToChannel[selectorID] = channel.id
		go c.readFrom(channel.id, channel.conn)
	}
	if c.unregisterObserver == nil {
		c.unregisterObserver = func() {}
	}
	if c.telemetryEvery > 0 {
		go c.logTelemetry()
	}
	return c
}

func (c *relayMultipathConn) ObservePacket(data []byte, outbound bool) {
	if !outbound || c.closed.Load() {
		return
	}
	info, err := multipath.ClassifyPacketInfo(data)
	if err != nil || !c.matchesAllowedDestination(info.Destination) {
		return
	}
	selected, err := multipath.SelectChannel(info.Flow, c.selectorSnapshot())
	if err != nil {
		return
	}
	channelID, ok := c.selectorIDToChannel[selected.ID]
	if !ok {
		return
	}
	c.enqueueHint(channelID)
}

func (c *relayMultipathConn) Read(b []byte) (int, error) {
	select {
	case result := <-c.readCh:
		if result.err != nil {
			return 0, result.err
		}
		n := copy(b, result.payload)
		if n < len(result.payload) {
			return n, io.ErrShortBuffer
		}
		return n, nil
	case <-c.done:
		if err := c.err(); err != nil {
			return 0, err
		}
		return 0, net.ErrClosed
	}
}

func (c *relayMultipathConn) Write(p []byte) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	channelID := c.primaryChannelID
	if isWireGuardDataPacket(p) {
		switch c.writeStrategy {
		case relayMultipathStrategyPacketBurst:
			channelID = c.nextStripedChannelID()
		default:
			select {
			case channelID = <-c.hintCh:
			default:
			}
		}
	}

	attempted := make(map[uint32]struct{}, len(c.channels))
	var lastErr error
	for len(attempted) < len(c.channels) {
		conn, selectedChannelID := c.connForWriteExcluding(channelID, attempted)
		if conn == nil {
			break
		}
		attempted[selectedChannelID] = struct{}{}
		c.recordChannelSelected(selectedChannelID)

		n, err := c.writeToChannel(selectedChannelID, conn, p)
		if err == nil {
			return n, nil
		}
		lastErr = err
		if errors.Is(err, errRelayMultipathChannelCongested) {
			channelID = c.primaryChannelID
			continue
		}
		c.markChannelUnhealthy(selectedChannelID, conn, err)
		if n > 0 {
			return n, err
		}

		channelID = c.primaryChannelID
	}

	if lastErr != nil {
		return 0, lastErr
	}
	return 0, net.ErrClosed
}

func (c *relayMultipathConn) writeToChannel(channelID uint32, conn net.Conn, p []byte) (int, error) {
	if c.batchingEnabled && isWireGuardDataPacket(p) {
		if err := c.batcherFor(channelID, conn).enqueue(p); err != nil {
			if errors.Is(err, errRelayMultipathChannelCongested) {
				c.markChannelCongested(channelID)
			}
			return 0, err
		}
		return len(p), nil
	}
	n, err := c.writeRaw(channelID, conn, p)
	if err != nil {
		return n, err
	}
	return n, nil
}

func (c *relayMultipathConn) Close() error {
	var result error
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.done)
		c.observerMu.Lock()
		unregisterObserver := c.unregisterObserver
		c.unregisterObserver = func() {}
		c.observerMu.Unlock()
		unregisterObserver()

		c.closeBatchers()
		conns := c.channelConnsSnapshot()
		for _, conn := range conns {
			if err := conn.Close(); err != nil && result == nil {
				result = err
			}
		}
	})
	return result
}

func (c *relayMultipathConn) setUnregisterObserver(unregisterObserver func()) {
	if unregisterObserver == nil {
		unregisterObserver = func() {}
	}

	c.observerMu.Lock()
	defer c.observerMu.Unlock()

	if c.closed.Load() {
		unregisterObserver()
		return
	}
	c.unregisterObserver = unregisterObserver
}

func (c *relayMultipathConn) setChannelReopener(reopener relayMultipathChannelReopener) {
	c.reopenMu.Lock()
	defer c.reopenMu.Unlock()
	c.reopener = reopener
}

func (c *relayMultipathConn) LocalAddr() net.Addr {
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	if conn := c.channels[c.primaryChannelID]; conn != nil {
		return conn.LocalAddr()
	}
	return nil
}

func (c *relayMultipathConn) RemoteAddr() net.Addr {
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	if conn := c.channels[c.primaryChannelID]; conn != nil {
		return conn.RemoteAddr()
	}
	return nil
}

func (c *relayMultipathConn) SetDeadline(t time.Time) error {
	var result error
	for _, conn := range c.channelConnsSnapshot() {
		if err := conn.SetDeadline(t); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func (c *relayMultipathConn) SetReadDeadline(t time.Time) error {
	var result error
	for _, conn := range c.channelConnsSnapshot() {
		if err := conn.SetReadDeadline(t); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func (c *relayMultipathConn) SetWriteDeadline(t time.Time) error {
	var result error
	for _, conn := range c.channelConnsSnapshot() {
		if err := conn.SetWriteDeadline(t); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func (c *relayMultipathConn) readFrom(channelID uint32, conn net.Conn) {
	buf := make([]byte, relayMultipathReadBuffer)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if c.closed.Load() {
				return
			}
			c.markChannelUnhealthy(channelID, conn, err)
			return
		}
		c.recordChannelRead(channelID, n)
		payload := make([]byte, n)
		copy(payload, buf[:n])
		if packets, ok := decodeRelayPacketBatch(payload); ok {
			for _, packet := range packets {
				if !c.enqueueRead(packet) {
					return
				}
			}
			continue
		}
		if !c.enqueueRead(payload) {
			return
		}
	}
}

func (c *relayMultipathConn) enqueueRead(payload []byte) bool {
	select {
	case c.readCh <- relayMultipathRead{payload: payload}:
		return true
	case <-c.done:
		return false
	}
}

func (c *relayMultipathConn) enqueueHint(channelID uint32) {
	select {
	case c.hintCh <- channelID:
	default:
		select {
		case <-c.hintCh:
		default:
		}
		select {
		case c.hintCh <- channelID:
		default:
		}
	}
}

func (c *relayMultipathConn) selectorSnapshot() []multipath.Channel {
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	channels := make([]multipath.Channel, len(c.selectorChannels))
	copy(channels, c.selectorChannels)
	return channels
}

func (c *relayMultipathConn) connForWrite(preferred uint32) (net.Conn, uint32) {
	return c.connForWriteExcluding(preferred, nil)
}

func (c *relayMultipathConn) connForWriteExcluding(preferred uint32, excluded map[uint32]struct{}) (net.Conn, uint32) {
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	candidates := c.writeCandidatesLocked(time.Now(), excluded, false, false, true)
	if len(candidates) == 0 {
		candidates = c.writeCandidatesLocked(time.Now(), excluded, true, true, true)
	}

	if conn, ok := relayMultipathCandidateConn(candidates, preferred); ok {
		return conn, preferred
	}
	if len(candidates) > 0 {
		return candidates[0].conn, candidates[0].id
	}
	return nil, 0
}

func channelExcluded(channelID uint32, excluded map[uint32]struct{}) bool {
	if len(excluded) == 0 {
		return false
	}
	_, ok := excluded[channelID]
	return ok
}

func (c *relayMultipathConn) nextStripedChannelID() uint32 {
	channelIDs := c.healthyChannelIDs()
	if len(channelIDs) == 0 {
		return c.primaryChannelID
	}
	packetIndex := c.stripeCounter.Add(1) - 1
	channelIndex := int((packetIndex / c.packetBurstSize) % uint64(len(channelIDs)))
	return channelIDs[channelIndex]
}

func (c *relayMultipathConn) healthyChannelIDs() []uint32 {
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	candidates := c.writeCandidatesLocked(time.Now(), nil, false, false, false)
	if len(candidates) == 0 {
		candidates = c.writeCandidatesLocked(time.Now(), nil, true, true, false)
	}
	channelIDs := make([]uint32, 0, len(candidates))
	for _, candidate := range candidates {
		channelIDs = append(channelIDs, candidate.id)
	}
	return channelIDs
}

func (c *relayMultipathConn) writeCandidatesLocked(now time.Time, excluded map[uint32]struct{}, allowCooling bool, allowOverLimit bool, sortByScore bool) []relayMultipathWriteCandidate {
	candidates := make([]relayMultipathWriteCandidate, 0, len(c.selectorChannels))
	for _, channel := range c.selectorChannels {
		if !channel.Healthy {
			continue
		}
		channelID, ok := c.selectorIDToChannel[channel.ID]
		if !ok || channelExcluded(channelID, excluded) {
			continue
		}
		conn := c.channels[channelID]
		if conn == nil {
			continue
		}
		if !allowCooling && c.channelCooling(channelID, now) {
			continue
		}
		if !allowOverLimit && c.channelOverInflightLimit(channelID) {
			continue
		}
		candidates = append(candidates, relayMultipathWriteCandidate{
			id:    channelID,
			conn:  conn,
			score: c.channelScore(channelID, now),
		})
	}
	if c.scoringEnabled && sortByScore {
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].score == candidates[j].score {
				return candidates[i].id < candidates[j].id
			}
			return candidates[i].score > candidates[j].score
		})
	}
	return candidates
}

func relayMultipathCandidateConn(candidates []relayMultipathWriteCandidate, channelID uint32) (net.Conn, bool) {
	for _, candidate := range candidates {
		if candidate.id == channelID {
			return candidate.conn, true
		}
	}
	return nil, false
}

func (c *relayMultipathConn) channelHealthyLocked(channelID uint32) bool {
	selectorID := strconv.FormatUint(uint64(channelID), 10)
	for _, channel := range c.selectorChannels {
		if channel.ID == selectorID {
			return channel.Healthy
		}
	}
	return false
}

func newRelayMultipathChannelStats(now time.Time) *relayMultipathChannelStats {
	stats := &relayMultipathChannelStats{}
	stats.reset(now)
	return stats
}

func (s *relayMultipathChannelStats) reset(now time.Time) {
	nowNano := now.UnixNano()
	s.lastReadUnixNano.Store(nowNano)
	s.lastWriteUnixNano.Store(nowNano)
	s.pendingWriteBytes.Store(0)
	s.activeWriteBytes.Store(0)
	s.writeEWMAMicros.Store(0)
	s.slowUntilUnixNano.Store(0)
	s.nextWriteUnixNano.Store(0)
	s.writeFailures.Store(0)
	s.selectedWrites.Store(0)
	s.readFrames.Store(0)
	s.readBytes.Store(0)
	s.writtenBytes.Store(0)
	s.congestions.Store(0)
}

func (c *relayMultipathConn) resetChannelStats(channelID uint32) {
	if stats := c.channelStats[channelID]; stats != nil {
		stats.reset(time.Now())
	}
}

func (c *relayMultipathConn) recordChannelSelected(channelID uint32) {
	if stats := c.channelStats[channelID]; stats != nil {
		stats.selectedWrites.Add(1)
	}
}

func (c *relayMultipathConn) recordChannelRead(channelID uint32, n int) {
	if stats := c.channelStats[channelID]; stats != nil {
		stats.lastReadUnixNano.Store(time.Now().UnixNano())
		stats.readFrames.Add(1)
		if n > 0 {
			stats.readBytes.Add(uint64(n))
		}
	}
}

func (c *relayMultipathConn) recordChannelWrite(channelID uint32, duration time.Duration, n int, err error) {
	stats := c.channelStats[channelID]
	if stats == nil {
		return
	}

	now := time.Now()
	stats.lastWriteUnixNano.Store(now.UnixNano())
	if err != nil {
		c.recordChannelFailure(channelID)
		return
	}
	if n <= 0 {
		return
	}
	stats.writtenBytes.Add(uint64(n))

	micros := duration.Microseconds()
	if micros < 1 {
		micros = 1
	}
	for {
		current := stats.writeEWMAMicros.Load()
		next := micros
		if current > 0 {
			next = (current*7 + micros) / 8
		}
		if stats.writeEWMAMicros.CompareAndSwap(current, next) {
			break
		}
	}
	if c.scoringEnabled && duration >= c.slowWrite {
		stats.slowUntilUnixNano.Store(now.Add(c.channelCooldown).UnixNano())
	}
	if c.scoringEnabled && c.pacingDelay > 0 {
		delay := c.pacingDelay
		if duration > c.pacingDelay {
			extra := duration / 8
			if extra > 25*time.Millisecond {
				extra = 25 * time.Millisecond
			}
			delay += extra
		}
		c.extendChannelNextWrite(channelID, now.Add(delay))
	}
}

func (c *relayMultipathConn) recordChannelFailure(channelID uint32) {
	stats := c.channelStats[channelID]
	if stats == nil {
		return
	}
	stats.writeFailures.Add(1)
	if c.scoringEnabled {
		stats.slowUntilUnixNano.Store(time.Now().Add(c.channelCooldown).UnixNano())
	}
}

func (c *relayMultipathConn) markChannelCongested(channelID uint32) {
	stats := c.channelStats[channelID]
	if stats == nil {
		return
	}
	stats.congestions.Add(1)
	if !c.scoringEnabled {
		return
	}
	stats.slowUntilUnixNano.Store(time.Now().Add(c.channelCooldown).UnixNano())
}

func (c *relayMultipathConn) tryReserveChannelPendingBytes(channelID uint32, bytes int64) bool {
	if bytes <= 0 {
		return true
	}
	stats := c.channelStats[channelID]
	if stats == nil {
		return true
	}
	stats.pendingWriteBytes.Add(bytes)
	if c.maxInflight <= 0 || c.channelInFlightBytes(channelID) <= c.maxInflight {
		return true
	}
	stats.pendingWriteBytes.Add(-bytes)
	return false
}

func (c *relayMultipathConn) releaseChannelPendingBytes(channelID uint32, bytes int64) {
	if bytes <= 0 {
		return
	}
	stats := c.channelStats[channelID]
	if stats == nil {
		return
	}
	stats.pendingWriteBytes.Add(-bytes)
}

func (c *relayMultipathConn) addChannelActiveWriteBytes(channelID uint32, bytes int64) {
	if bytes <= 0 {
		return
	}
	if stats := c.channelStats[channelID]; stats != nil {
		stats.activeWriteBytes.Add(bytes)
	}
}

func (c *relayMultipathConn) releaseChannelActiveWriteBytes(channelID uint32, bytes int64) {
	if bytes <= 0 {
		return
	}
	if stats := c.channelStats[channelID]; stats != nil {
		stats.activeWriteBytes.Add(-bytes)
	}
}

func (c *relayMultipathConn) channelInFlightBytes(channelID uint32) int64 {
	stats := c.channelStats[channelID]
	if stats == nil {
		return 0
	}
	return stats.pendingWriteBytes.Load() + stats.activeWriteBytes.Load()
}

func (c *relayMultipathConn) channelOverInflightLimit(channelID uint32) bool {
	return c.maxInflight > 0 && c.channelInFlightBytes(channelID) >= c.maxInflight
}

func (c *relayMultipathConn) channelPacingWait(channelID uint32, now time.Time) time.Duration {
	if !c.scoringEnabled || c.pacingDelay <= 0 {
		return 0
	}
	stats := c.channelStats[channelID]
	if stats == nil {
		return 0
	}
	next := stats.nextWriteUnixNano.Load()
	if next <= now.UnixNano() {
		return 0
	}
	return time.Until(time.Unix(0, next))
}

func (c *relayMultipathConn) extendChannelNextWrite(channelID uint32, next time.Time) {
	stats := c.channelStats[channelID]
	if stats == nil {
		return
	}
	nextNano := next.UnixNano()
	for {
		current := stats.nextWriteUnixNano.Load()
		if current >= nextNano {
			return
		}
		if stats.nextWriteUnixNano.CompareAndSwap(current, nextNano) {
			return
		}
	}
}

func (c *relayMultipathConn) batchLimits(channelID uint32) (int, int) {
	maxPackets := relayMultipathBatchMaxPackets
	maxBytes := relayMultipathBatchMaxBytes
	if !c.scoringEnabled {
		return maxPackets, maxBytes
	}

	stats := c.channelStats[channelID]
	if stats == nil {
		return maxPackets, maxBytes
	}
	if c.maxInflight > 0 && c.channelInFlightBytes(channelID) > c.maxInflight/2 {
		maxPackets = 2
		maxBytes = 4096
	}
	if c.slowWrite > 0 && stats.writeEWMAMicros.Load() > (c.slowWrite.Microseconds()/2) {
		maxPackets = 2
		if maxBytes > 4096 {
			maxBytes = 4096
		}
	}
	if c.readIdlePenalty > 0 {
		lastRead := stats.lastReadUnixNano.Load()
		if lastRead > 0 && time.Since(time.Unix(0, lastRead)) > c.readIdlePenalty/2 {
			maxPackets = 1
			maxBytes = 2048
		}
	}
	return maxPackets, maxBytes
}

func (c *relayMultipathConn) channelCooling(channelID uint32, now time.Time) bool {
	if !c.scoringEnabled {
		return false
	}
	stats := c.channelStats[channelID]
	return stats != nil && stats.slowUntilUnixNano.Load() > now.UnixNano()
}

func (c *relayMultipathConn) channelScore(channelID uint32, now time.Time) int64 {
	if !c.scoringEnabled {
		return 0
	}
	const baseScore = int64(1_000_000_000)
	score := baseScore
	stats := c.channelStats[channelID]
	if stats == nil {
		return score
	}

	score -= stats.writeEWMAMicros.Load() * 100
	score -= int64(stats.writeFailures.Load()) * 1_000_000
	score -= c.channelInFlightBytes(channelID) * 1000
	if slowUntil := stats.slowUntilUnixNano.Load(); slowUntil > now.UnixNano() {
		score -= 1_000_000_000_000
	}
	if lastRead := stats.lastReadUnixNano.Load(); lastRead > 0 && c.readIdlePenalty > 0 {
		idle := now.Sub(time.Unix(0, lastRead))
		if idle > c.readIdlePenalty {
			score -= int64((idle-c.readIdlePenalty)/time.Millisecond) * 1000
		}
	}
	return score
}

func (c *relayMultipathConn) logTelemetry() {
	ticker := time.NewTicker(c.telemetryEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.logTelemetrySnapshot()
		case <-c.done:
			return
		}
	}
}

func (c *relayMultipathConn) logTelemetrySnapshot() {
	now := time.Now()
	channels := c.telemetrySnapshot(now)
	if len(channels) == 0 {
		return
	}

	var b strings.Builder
	_, _ = fmt.Fprintf(
		&b,
		"anonymous relay multipath telemetry strategy=%s batch=%t scoring=%t burst=%d pacing=%s max_inflight=%d channels=",
		c.writeStrategy,
		c.batchingEnabled,
		c.scoringEnabled,
		c.packetBurstSize,
		c.pacingDelay,
		c.maxInflight,
	)
	for i, channel := range channels {
		if i > 0 {
			b.WriteByte(' ')
		}
		_, _ = fmt.Fprintf(
			&b,
			"{id=%d healthy=%t pending=%d active=%d ewma=%s cooldown=%s pace_wait=%s read_idle=%s write_idle=%s queue=%d selected=%d read_frames=%d read_bytes=%d written_bytes=%d failures=%d congestions=%d}",
			channel.id,
			channel.healthy,
			channel.pendingBytes,
			channel.activeBytes,
			channel.writeEWMA,
			channel.cooldown,
			channel.pacingWait,
			channel.readIdle,
			channel.writeIdle,
			channel.queueLen,
			channel.selectedWrites,
			channel.readFrames,
			channel.readBytes,
			channel.writtenBytes,
			channel.writeFailures,
			channel.congestions,
		)
	}
	log.Info(b.String())
}

func (c *relayMultipathConn) telemetrySnapshot(now time.Time) []relayMultipathTelemetryChannel {
	c.selectorMu.RLock()
	channels := make([]relayMultipathTelemetryChannel, 0, len(c.selectorChannels))
	for _, selectorChannel := range c.selectorChannels {
		channelID, ok := c.selectorIDToChannel[selectorChannel.ID]
		if !ok {
			continue
		}
		stats := c.channelStats[channelID]
		snapshot := relayMultipathTelemetryChannel{
			id:      channelID,
			healthy: selectorChannel.Healthy,
		}
		if stats != nil {
			lastRead := stats.lastReadUnixNano.Load()
			if lastRead > 0 {
				snapshot.readIdle = now.Sub(time.Unix(0, lastRead)).Truncate(time.Millisecond)
			}
			lastWrite := stats.lastWriteUnixNano.Load()
			if lastWrite > 0 {
				snapshot.writeIdle = now.Sub(time.Unix(0, lastWrite)).Truncate(time.Millisecond)
			}
			slowUntil := stats.slowUntilUnixNano.Load()
			if slowUntil > now.UnixNano() {
				snapshot.cooldown = time.Until(time.Unix(0, slowUntil)).Truncate(time.Millisecond)
			}
			nextWrite := stats.nextWriteUnixNano.Load()
			if nextWrite > now.UnixNano() {
				snapshot.pacingWait = time.Until(time.Unix(0, nextWrite)).Truncate(time.Millisecond)
			}
			snapshot.pendingBytes = stats.pendingWriteBytes.Load()
			snapshot.activeBytes = stats.activeWriteBytes.Load()
			snapshot.writeEWMA = (time.Duration(stats.writeEWMAMicros.Load()) * time.Microsecond).Truncate(time.Microsecond)
			snapshot.selectedWrites = stats.selectedWrites.Load()
			snapshot.readFrames = stats.readFrames.Load()
			snapshot.readBytes = stats.readBytes.Load()
			snapshot.writtenBytes = stats.writtenBytes.Load()
			snapshot.writeFailures = stats.writeFailures.Load()
			snapshot.congestions = stats.congestions.Load()
		}
		channels = append(channels, snapshot)
	}
	c.selectorMu.RUnlock()

	c.batchMu.Lock()
	for i := range channels {
		if batcher := c.batchers[channels[i].id]; batcher != nil {
			channels[i].queueLen = len(batcher.queue)
		}
	}
	c.batchMu.Unlock()
	return channels
}

func (c *relayMultipathConn) matchesAllowedDestination(dst netip.Addr) bool {
	dst = dst.Unmap()
	for _, prefix := range c.allowedIPs {
		if prefix.Contains(dst) {
			return true
		}
	}
	return false
}

func (c *relayMultipathConn) fail(err error) {
	c.errMu.Lock()
	if c.readErr == nil {
		c.readErr = err
	}
	c.errMu.Unlock()

	select {
	case c.readCh <- relayMultipathRead{err: err}:
	default:
	}
	_ = c.Close()
}

func (c *relayMultipathConn) markChannelUnhealthy(channelID uint32, failedConn net.Conn, err error) {
	var remaining int
	var conn net.Conn
	var changed bool

	c.selectorMu.Lock()
	conn = c.channels[channelID]
	if failedConn != nil && conn != failedConn {
		c.selectorMu.Unlock()
		return
	}
	c.recordChannelFailure(channelID)
	selectorID := strconv.FormatUint(uint64(channelID), 10)
	for i := range c.selectorChannels {
		if c.selectorChannels[i].ID == selectorID {
			changed = c.selectorChannels[i].Healthy
			c.selectorChannels[i].Healthy = false
			break
		}
	}
	for _, channel := range c.selectorChannels {
		if channel.Healthy {
			remaining++
		}
	}
	c.selectorMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	c.closeBatcher(channelID)
	if changed {
		c.startChannelReopen(channelID)
	}
	if remaining == 0 && !c.hasChannelReopener() {
		c.fail(err)
	}
}

func (c *relayMultipathConn) channelConnsSnapshot() []net.Conn {
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	conns := make([]net.Conn, 0, len(c.channels))
	for _, conn := range c.channels {
		if conn != nil {
			conns = append(conns, conn)
		}
	}
	return conns
}

func (c *relayMultipathConn) hasChannelReopener() bool {
	c.reopenMu.Lock()
	defer c.reopenMu.Unlock()
	return c.reopener != nil
}

func (c *relayMultipathConn) startChannelReopen(channelID uint32) {
	c.reopenMu.Lock()
	reopener := c.reopener
	if reopener == nil || c.closed.Load() {
		c.reopenMu.Unlock()
		return
	}
	if _, ok := c.reopening[channelID]; ok {
		c.reopenMu.Unlock()
		return
	}
	c.reopening[channelID] = struct{}{}
	c.reopenMu.Unlock()

	go c.reopenChannel(channelID, reopener)
}

func (c *relayMultipathConn) reopenChannel(channelID uint32, reopener relayMultipathChannelReopener) {
	defer func() {
		c.reopenMu.Lock()
		delete(c.reopening, channelID)
		c.reopenMu.Unlock()
	}()

	backoff := relayMultipathReopenInitialBackoff
	for !c.closed.Load() {
		ctx, cancel := context.WithTimeout(context.Background(), relayMultipathReopenTimeout)
		conn, err := reopener(ctx, channelID)
		cancel()
		if err == nil && conn != nil {
			if c.installReopenedChannel(channelID, conn) {
				return
			}
			_ = conn.Close()
			return
		}

		select {
		case <-c.done:
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > relayMultipathReopenMaxBackoff {
			backoff = relayMultipathReopenMaxBackoff
		}
	}
}

func (c *relayMultipathConn) installReopenedChannel(channelID uint32, conn net.Conn) bool {
	if c.closed.Load() {
		return false
	}

	c.selectorMu.Lock()
	defer c.selectorMu.Unlock()

	if c.closed.Load() {
		return false
	}
	selectorID := strconv.FormatUint(uint64(channelID), 10)
	for i := range c.selectorChannels {
		if c.selectorChannels[i].ID == selectorID {
			c.closeBatcher(channelID)
			c.channels[channelID] = conn
			c.selectorChannels[i].Healthy = true
			c.resetChannelStats(channelID)
			go c.readFrom(channelID, conn)
			return true
		}
	}
	return false
}

func (c *relayMultipathConn) batcherFor(channelID uint32, conn net.Conn) *relayMultipathBatcher {
	c.batchMu.Lock()
	defer c.batchMu.Unlock()

	if batcher := c.batchers[channelID]; batcher != nil && batcher.conn == conn {
		return batcher
	}
	if batcher := c.batchers[channelID]; batcher != nil {
		batcher.close()
	}
	batcher := newRelayMultipathBatcher(c, channelID, conn)
	c.batchers[channelID] = batcher
	return batcher
}

func (c *relayMultipathConn) closeBatcher(channelID uint32) {
	c.batchMu.Lock()
	batcher := c.batchers[channelID]
	delete(c.batchers, channelID)
	c.batchMu.Unlock()
	if batcher != nil {
		batcher.close()
	}
}

func (c *relayMultipathConn) closeBatchers() {
	c.batchMu.Lock()
	batchers := c.batchers
	c.batchers = make(map[uint32]*relayMultipathBatcher)
	c.batchMu.Unlock()
	for _, batcher := range batchers {
		batcher.close()
	}
}

func (c *relayMultipathConn) writeRaw(channelID uint32, conn net.Conn, p []byte) (int, error) {
	lock := c.writeLock(channelID)
	lock.Lock()
	defer lock.Unlock()
	writeBytes := int64(len(p))
	c.addChannelActiveWriteBytes(channelID, writeBytes)
	defer c.releaseChannelActiveWriteBytes(channelID, writeBytes)
	start := time.Now()
	n, err := conn.Write(p)
	c.recordChannelWrite(channelID, time.Since(start), n, err)
	return n, err
}

func (c *relayMultipathConn) writeLock(channelID uint32) *sync.Mutex {
	c.batchMu.Lock()
	defer c.batchMu.Unlock()
	lock := c.writeLocks[channelID]
	if lock == nil {
		lock = &sync.Mutex{}
		c.writeLocks[channelID] = lock
	}
	return lock
}

type relayMultipathBatcher struct {
	parent  *relayMultipathConn
	conn    net.Conn
	channel uint32
	queue   chan []byte
	done    chan struct{}
	mu      sync.Mutex
	closed  bool
	once    sync.Once
}

func newRelayMultipathBatcher(parent *relayMultipathConn, channelID uint32, conn net.Conn) *relayMultipathBatcher {
	b := &relayMultipathBatcher{
		parent:  parent,
		conn:    conn,
		channel: channelID,
		queue:   make(chan []byte, relayMultipathBatchQueueSize),
		done:    make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *relayMultipathBatcher) enqueue(packet []byte) error {
	copied := append([]byte(nil), packet...)
	packetBytes := relayMultipathPacketAccountedBytes(copied)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return net.ErrClosed
	}
	if !b.parent.tryReserveChannelPendingBytes(b.channel, packetBytes) {
		return errRelayMultipathChannelCongested
	}
	select {
	case b.queue <- copied:
		return nil
	case <-b.done:
		b.parent.releaseChannelPendingBytes(b.channel, packetBytes)
		return net.ErrClosed
	default:
		b.parent.releaseChannelPendingBytes(b.channel, packetBytes)
		return errRelayMultipathChannelCongested
	}
}

func (b *relayMultipathBatcher) close() {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		close(b.done)
		b.mu.Unlock()
	})
}

func (b *relayMultipathBatcher) run() {
	var batch [][]byte
	var batchBytes int
	timer := time.NewTimer(relayMultipathBatchFlushDelay)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	waitForPacing := func() bool {
		wait := b.parent.channelPacingWait(b.channel, time.Now())
		if wait <= 0 {
			return true
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
			return true
		case <-b.done:
			return false
		}
	}
	flush := func(pace bool) bool {
		if len(batch) == 0 {
			return true
		}
		accountedBytes := relayMultipathBatchAccountedBytes(batch)
		if pace && !waitForPacing() {
			batch = nil
			batchBytes = 0
			b.parent.releaseChannelPendingBytes(b.channel, accountedBytes)
			b.drainQueue()
			return false
		}
		payload := encodeRelayPacketBatch(batch)
		batch = nil
		batchBytes = 0
		b.parent.releaseChannelPendingBytes(b.channel, accountedBytes)
		if _, err := b.parent.writeRaw(b.channel, b.conn, payload); err != nil {
			b.parent.markChannelUnhealthy(b.channel, b.conn, err)
			return false
		}
		return true
	}
	addPacket := func(packet []byte) {
		if len(batch) == 0 {
			batchBytes = relayMultipathBatchHeaderSize
		}
		batch = append(batch, packet)
		batchBytes += relayMultipathBatchPacketHeader + len(packet)
	}

	for {
		if len(batch) == 0 {
			select {
			case packet := <-b.queue:
				addPacket(packet)
				timer.Reset(relayMultipathBatchFlushDelay)
				maxPackets, maxBytes := b.parent.batchLimits(b.channel)
				if len(batch) >= maxPackets || batchBytes >= maxBytes {
					if !flush(true) {
						return
					}
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				}
			case <-b.done:
				b.drainQueue()
				return
			}
		}

		select {
		case packet := <-b.queue:
			maxPackets, maxBytes := b.parent.batchLimits(b.channel)
			packetBytes := relayMultipathBatchPacketHeader + len(packet)
			if len(batch) >= maxPackets || batchBytes+packetBytes > maxBytes {
				if !flush(true) {
					return
				}
				timer.Reset(relayMultipathBatchFlushDelay)
			}
			addPacket(packet)
			maxPackets, maxBytes = b.parent.batchLimits(b.channel)
			if len(batch) >= maxPackets || batchBytes >= maxBytes {
				if !flush(true) {
					return
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
		case <-timer.C:
			if !flush(true) {
				return
			}
		case <-b.done:
			_ = flush(false)
			b.drainQueue()
			return
		}
	}
}

func (b *relayMultipathBatcher) drainQueue() {
	for {
		select {
		case packet := <-b.queue:
			b.parent.releaseChannelPendingBytes(b.channel, relayMultipathPacketAccountedBytes(packet))
		default:
			return
		}
	}
}

func encodeRelayPacketBatch(packets [][]byte) []byte {
	if len(packets) == 1 {
		return packets[0]
	}
	size := relayMultipathBatchHeaderSize
	for _, packet := range packets {
		size += relayMultipathBatchPacketHeader + len(packet)
	}
	out := make([]byte, size)
	copy(out[:4], relayMultipathBatchMagic[:])
	binary.BigEndian.PutUint16(out[4:6], uint16(len(packets)))
	offset := relayMultipathBatchHeaderSize
	for _, packet := range packets {
		binary.BigEndian.PutUint16(out[offset:offset+2], uint16(len(packet)))
		offset += relayMultipathBatchPacketHeader
		copy(out[offset:offset+len(packet)], packet)
		offset += len(packet)
	}
	return out
}

func relayMultipathPacketAccountedBytes(packet []byte) int64 {
	return int64(relayMultipathBatchPacketHeader + len(packet))
}

func relayMultipathBatchAccountedBytes(packets [][]byte) int64 {
	var bytes int64
	for _, packet := range packets {
		bytes += relayMultipathPacketAccountedBytes(packet)
	}
	return bytes
}

func decodeRelayPacketBatch(payload []byte) ([][]byte, bool) {
	if len(payload) < relayMultipathBatchHeaderSize || string(payload[:4]) != string(relayMultipathBatchMagic[:]) {
		return nil, false
	}
	count := int(binary.BigEndian.Uint16(payload[4:6]))
	if count < 2 {
		return nil, false
	}
	offset := relayMultipathBatchHeaderSize
	packets := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		if len(payload)-offset < relayMultipathBatchPacketHeader {
			return nil, false
		}
		size := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
		offset += relayMultipathBatchPacketHeader
		if size == 0 || len(payload)-offset < size {
			return nil, false
		}
		packet := make([]byte, size)
		copy(packet, payload[offset:offset+size])
		offset += size
		packets = append(packets, packet)
	}
	if offset != len(payload) {
		return nil, false
	}
	return packets, true
}

func (c *relayMultipathConn) err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.readErr
}

func isWireGuardDataPacket(packet []byte) bool {
	return len(packet) >= 4 && packet[0] == 4 && packet[1] == 0 && packet[2] == 0 && packet[3] == 0
}

func relayMultipathWriteStrategy() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAnonRelayMultipathStrategy))) {
	case "", relayMultipathStrategyFlowAffine:
		return relayMultipathStrategyFlowAffine
	case relayMultipathStrategyPacketBurst, "packet-stripe", "stripe":
		return relayMultipathStrategyPacketBurst
	default:
		return relayMultipathStrategyFlowAffine
	}
}

func relayMultipathPacketBurstSize() uint64 {
	raw := strings.TrimSpace(os.Getenv(envAnonRelayMultipathPacketBurstSize))
	if raw == "" {
		return relayMultipathDefaultPacketBurstSize
	}
	size, err := strconv.Atoi(raw)
	if err != nil || size < 1 {
		return relayMultipathDefaultPacketBurstSize
	}
	if size > relayMultipathMaxPacketBurstSize {
		return relayMultipathMaxPacketBurstSize
	}
	return uint64(size)
}

func relayMultipathBatchingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAnonRelayMultipathBatch))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func relayMultipathChannelScoringEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAnonRelayMultipathScoring))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func relayMultipathDurationFromMS(envName string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fallback
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 1 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func relayMultipathDurationFromMSAllowZero(envName string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "0" {
		return 0
	}
	return relayMultipathDurationFromMS(envName, fallback)
}

func relayMultipathInt64FromEnv(envName string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func relayMultipathInt64FromEnvAllowZero(envName string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "0" {
		return 0
	}
	return relayMultipathInt64FromEnv(envName, fallback)
}
