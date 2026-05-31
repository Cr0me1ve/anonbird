package peer

import (
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netbirdio/netbird/client/internal/anonymous/multipath"
)

const (
	relayMultipathHintQueueSize = 256
	relayMultipathReadQueueSize = 64
	relayMultipathReadBuffer    = 65535
)

type relayMultipathChannel struct {
	id   uint32
	conn net.Conn
}

type relayMultipathRead struct {
	payload []byte
	err     error
}

type relayMultipathConn struct {
	channels            map[uint32]net.Conn
	selectorChannels    []multipath.Channel
	selectorIDToChannel map[string]uint32
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
}

func newRelayMultipathConn(channels []relayMultipathChannel, allowedIPs []netip.Prefix, unregisterObserver func()) *relayMultipathConn {
	c := &relayMultipathConn{
		channels:            make(map[uint32]net.Conn, len(channels)),
		selectorChannels:    make([]multipath.Channel, 0, len(channels)),
		selectorIDToChannel: make(map[string]uint32, len(channels)),
		allowedIPs:          append([]netip.Prefix(nil), allowedIPs...),
		hintCh:              make(chan uint32, relayMultipathHintQueueSize),
		readCh:              make(chan relayMultipathRead, relayMultipathReadQueueSize),
		done:                make(chan struct{}),
		unregisterObserver:  unregisterObserver,
	}
	for i, channel := range channels {
		if i == 0 {
			c.primaryChannelID = channel.id
		}
		c.channels[channel.id] = channel.conn
		selectorID := strconv.FormatUint(uint64(channel.id), 10)
		c.selectorChannels = append(c.selectorChannels, multipath.Channel{ID: selectorID, Healthy: true})
		c.selectorIDToChannel[selectorID] = channel.id
		go c.readFrom(channel.id, channel.conn)
	}
	if c.unregisterObserver == nil {
		c.unregisterObserver = func() {}
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
		select {
		case channelID = <-c.hintCh:
		default:
		}
	}
	conn, channelID := c.connForWrite(channelID)
	if conn == nil {
		return 0, net.ErrClosed
	}
	n, err := conn.Write(p)
	if err != nil {
		c.markChannelUnhealthy(channelID, err)
	}
	return n, err
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
		for _, conn := range c.channels {
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

func (c *relayMultipathConn) LocalAddr() net.Addr {
	if conn := c.channels[c.primaryChannelID]; conn != nil {
		return conn.LocalAddr()
	}
	return nil
}

func (c *relayMultipathConn) RemoteAddr() net.Addr {
	if conn := c.channels[c.primaryChannelID]; conn != nil {
		return conn.RemoteAddr()
	}
	return nil
}

func (c *relayMultipathConn) SetDeadline(t time.Time) error {
	var result error
	for _, conn := range c.channels {
		if err := conn.SetDeadline(t); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func (c *relayMultipathConn) SetReadDeadline(t time.Time) error {
	var result error
	for _, conn := range c.channels {
		if err := conn.SetReadDeadline(t); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func (c *relayMultipathConn) SetWriteDeadline(t time.Time) error {
	var result error
	for _, conn := range c.channels {
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
			c.markChannelUnhealthy(channelID, err)
			return
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		select {
		case c.readCh <- relayMultipathRead{payload: payload}:
		case <-c.done:
			return
		}
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
	c.selectorMu.RLock()
	defer c.selectorMu.RUnlock()

	if c.channelHealthyLocked(preferred) {
		if conn := c.channels[preferred]; conn != nil {
			return conn, preferred
		}
	}
	if c.channelHealthyLocked(c.primaryChannelID) {
		if conn := c.channels[c.primaryChannelID]; conn != nil {
			return conn, c.primaryChannelID
		}
	}
	for _, channel := range c.selectorChannels {
		if !channel.Healthy {
			continue
		}
		channelID, ok := c.selectorIDToChannel[channel.ID]
		if !ok {
			continue
		}
		if conn := c.channels[channelID]; conn != nil {
			return conn, channelID
		}
	}
	return nil, 0
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

func (c *relayMultipathConn) markChannelUnhealthy(channelID uint32, err error) {
	var remaining int
	var conn net.Conn

	c.selectorMu.Lock()
	selectorID := strconv.FormatUint(uint64(channelID), 10)
	for i := range c.selectorChannels {
		if c.selectorChannels[i].ID == selectorID {
			c.selectorChannels[i].Healthy = false
			break
		}
	}
	for _, channel := range c.selectorChannels {
		if channel.Healthy {
			remaining++
		}
	}
	conn = c.channels[channelID]
	c.selectorMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if remaining == 0 {
		c.fail(err)
	}
}

func (c *relayMultipathConn) err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.readErr
}

func isWireGuardDataPacket(packet []byte) bool {
	return len(packet) >= 4 && packet[0] == 4 && packet[1] == 0 && packet[2] == 0 && packet[3] == 0
}
