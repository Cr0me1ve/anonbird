package peer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
)

const anonymousI2PDatagramNetwork = "i2p-datagram"

type RemoteAnonymousTransport struct {
	Type           string
	I2PDestination string
}

type i2pDatagramSession interface {
	Send(ctx context.Context, destination string, payload []byte, opts i2psam.SendOptions) error
	Receive(ctx context.Context) (i2psam.Datagram, error)
	Close() error
}

type i2pDatagramSessionFactory func(ctx context.Context, cfg i2psam.DatagramConfig) (i2pDatagramSession, error)

var defaultI2PDatagramRegistry = newI2PDatagramRegistry(func(ctx context.Context, cfg i2psam.DatagramConfig) (i2pDatagramSession, error) {
	return i2psam.NewDatagramSession(ctx, cfg)
})

type i2pDatagramRegistry struct {
	mu         sync.Mutex
	managers   map[string]*i2pDatagramManager
	newSession i2pDatagramSessionFactory
}

func newI2PDatagramRegistry(factory i2pDatagramSessionFactory) *i2pDatagramRegistry {
	return &i2pDatagramRegistry{
		managers:   make(map[string]*i2pDatagramManager),
		newSession: factory,
	}
}

func (r *i2pDatagramRegistry) OpenPeerConn(ctx context.Context, transport anonymous.TransportConfig, remoteKey, remoteDestination string) (net.Conn, error) {
	transport = anonymous.NormalizeTransport(transport)
	if transport.Type != anonymous.TransportI2PDatagram {
		return nil, fmt.Errorf("anonymous transport %q does not support direct i2p datagrams", transport.Type)
	}
	if strings.TrimSpace(transport.I2PDestinationPrivate) == "" {
		return nil, errors.New("local i2p private destination is empty")
	}
	remoteDestination = strings.TrimSpace(remoteDestination)
	if remoteDestination == "" {
		return nil, errors.New("remote i2p destination is empty")
	}
	if err := anonymous.ValidateTransport(transport); err != nil {
		return nil, err
	}

	key := i2pDatagramManagerKey(transport)
	manager, err := r.retainManager(ctx, key, transport)
	if err != nil {
		return nil, err
	}

	conn, err := manager.register(remoteKey, remoteDestination)
	if err != nil {
		r.releaseManager(manager)
		return nil, err
	}
	return conn, nil
}

func (r *i2pDatagramRegistry) retainManager(ctx context.Context, key string, transport anonymous.TransportConfig) (*i2pDatagramManager, error) {
	r.mu.Lock()
	manager := r.managers[key]
	if manager != nil && !manager.isClosed() {
		manager.refs++
		r.mu.Unlock()
		return manager, nil
	}
	r.mu.Unlock()

	sessionID := "anonbird-wg-" + key[:16]
	session, err := r.newSession(ctx, i2psam.DatagramConfig{
		SAMAddress:       transport.I2PSAM,
		Style:            i2psam.DatagramStyleRepliable,
		SessionID:        sessionID,
		PrivateKey:       transport.I2PDestinationPrivate,
		InboundLength:    transport.I2PTunnelLength,
		OutboundLength:   transport.I2PTunnelLength,
		InboundQuantity:  transport.I2PTunnelQuantity,
		OutboundQuantity: transport.I2PTunnelQuantity,
	})
	if err != nil {
		return nil, err
	}

	manager = newI2PDatagramManager(key, session, r.releaseManager)

	r.mu.Lock()
	existing := r.managers[key]
	if existing != nil && !existing.isClosed() {
		existing.refs++
		r.mu.Unlock()
		_ = session.Close()
		manager.cancel()
		return existing, nil
	}
	manager.refs = 1
	r.managers[key] = manager
	r.mu.Unlock()

	manager.start()
	return manager, nil
}

func (r *i2pDatagramRegistry) releaseManager(manager *i2pDatagramManager) {
	if manager == nil {
		return
	}

	var closeManager bool
	r.mu.Lock()
	if manager.refs > 0 {
		manager.refs--
	}
	if manager.refs == 0 && r.managers[manager.key] == manager {
		delete(r.managers, manager.key)
		closeManager = true
	}
	r.mu.Unlock()

	if closeManager {
		manager.close()
	}
}

func i2pDatagramManagerKey(transport anonymous.TransportConfig) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		transport.I2PSAM,
		transport.I2PDestinationPrivate,
		strconv.Itoa(int(transport.I2PTunnelLength)),
		strconv.Itoa(int(transport.I2PTunnelQuantity)),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

type i2pDatagramManager struct {
	key     string
	session i2pDatagramSession
	release func(*i2pDatagramManager)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.RWMutex
	closed bool
	peers  map[string]*i2pDatagramPeerConn

	refs int
}

func newI2PDatagramManager(key string, session i2pDatagramSession, release func(*i2pDatagramManager)) *i2pDatagramManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &i2pDatagramManager{
		key:     key,
		session: session,
		release: release,
		ctx:     ctx,
		cancel:  cancel,
		peers:   make(map[string]*i2pDatagramPeerConn),
	}
}

func (m *i2pDatagramManager) start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.receiveLoop()
	}()
}

func (m *i2pDatagramManager) register(remoteKey, remoteDestination string) (*i2pDatagramPeerConn, error) {
	remoteDestination = strings.TrimSpace(remoteDestination)
	if remoteDestination == "" {
		return nil, errors.New("remote i2p destination is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, net.ErrClosed
	}
	if _, ok := m.peers[remoteDestination]; ok {
		return nil, fmt.Errorf("i2p datagram peer connection already registered for %s", shortI2PDestination(remoteDestination))
	}

	conn := &i2pDatagramPeerConn{
		manager:           m,
		remoteKey:         strings.TrimSpace(remoteKey),
		remoteDestination: remoteDestination,
		localAddr:         i2pDatagramAddr{destination: "local-" + m.key[:16]},
		remoteAddr:        i2pDatagramAddr{destination: remoteDestination},
		readCh:            make(chan []byte, 256),
		done:              make(chan struct{}),
		readErr:           io.EOF,
	}
	m.peers[remoteDestination] = conn
	return conn, nil
}

func (m *i2pDatagramManager) unregister(conn *i2pDatagramPeerConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.peers[conn.remoteDestination]; current == conn {
		delete(m.peers, conn.remoteDestination)
	}
}

func (m *i2pDatagramManager) send(ctx context.Context, destination string, payload []byte) error {
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return net.ErrClosed
	}
	return m.session.Send(ctx, destination, payload, i2psam.SendOptions{})
}

func (m *i2pDatagramManager) receiveLoop() {
	defer m.closePeers(io.EOF)

	for {
		datagram, err := m.session.Receive(m.ctx)
		if err != nil {
			if m.ctx.Err() != nil || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				return
			}
			log.Warnf("i2p datagram receive loop stopped: %v", err)
			return
		}

		source := strings.TrimSpace(datagram.Source)
		if source == "" {
			log.Debugf("dropping i2p datagram without source destination")
			continue
		}
		m.mu.RLock()
		conn := m.peers[source]
		m.mu.RUnlock()
		if conn == nil {
			log.Tracef("dropping i2p datagram from unregistered destination %s", shortI2PDestination(source))
			continue
		}
		conn.deliver(datagram.Payload)
	}
}

func (m *i2pDatagramManager) closePeers(err error) {
	m.mu.Lock()
	m.closed = true
	peers := make([]*i2pDatagramPeerConn, 0, len(m.peers))
	for _, conn := range m.peers {
		peers = append(peers, conn)
	}
	m.mu.Unlock()

	for _, conn := range peers {
		conn.closeWithError(err)
	}
}

func (m *i2pDatagramManager) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()

	m.cancel()
	_ = m.session.Close()
	m.wg.Wait()
	m.closePeers(net.ErrClosed)
}

func (m *i2pDatagramManager) isClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

type i2pDatagramPeerConn struct {
	manager           *i2pDatagramManager
	remoteKey         string
	remoteDestination string
	localAddr         net.Addr
	remoteAddr        net.Addr

	readCh      chan []byte
	done        chan struct{}
	once        sync.Once
	releaseOnce sync.Once

	deadlineMu    sync.RWMutex
	readDeadline  time.Time
	writeDeadline time.Time

	readErrMu sync.RWMutex
	readErr   error
}

func (c *i2pDatagramPeerConn) Read(p []byte) (int, error) {
	timer, timeout, err := c.deadlineTimer(c.getReadDeadline())
	if err != nil {
		return 0, err
	}
	if timer != nil {
		defer timer.Stop()
	}

	select {
	case payload := <-c.readCh:
		if len(payload) > len(p) {
			copy(p, payload[:len(p)])
			return len(p), io.ErrShortBuffer
		}
		copy(p, payload)
		return len(payload), nil
	case <-c.done:
		return 0, c.getReadErr()
	case <-timeout:
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *i2pDatagramPeerConn) Write(p []byte) (int, error) {
	select {
	case <-c.done:
		return 0, net.ErrClosed
	default:
	}
	if len(p) == 0 {
		return 0, nil
	}

	payload := append([]byte(nil), p...)
	ctx := context.Background()
	cancel := func() {}
	if deadline := c.getWriteDeadline(); !deadline.IsZero() {
		if !time.Now().Before(deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		ctx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()

	if err := c.manager.send(ctx, c.remoteDestination, payload); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *i2pDatagramPeerConn) Close() error {
	c.closeWithError(net.ErrClosed)
	c.releaseOnce.Do(func() {
		c.manager.unregister(c)
		c.manager.release(c.manager)
	})
	return nil
}

func (c *i2pDatagramPeerConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *i2pDatagramPeerConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *i2pDatagramPeerConn) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

func (c *i2pDatagramPeerConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	return nil
}

func (c *i2pDatagramPeerConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.writeDeadline = t
	return nil
}

func (c *i2pDatagramPeerConn) deliver(payload []byte) {
	packet := append([]byte(nil), payload...)
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case <-c.done:
	case c.readCh <- packet:
	default:
		log.Debugf("dropping i2p datagram for peer %s because receive queue is full", c.remoteKey)
	}
}

func (c *i2pDatagramPeerConn) closeWithError(err error) {
	c.once.Do(func() {
		if err == nil {
			err = io.EOF
		}
		c.readErrMu.Lock()
		c.readErr = err
		c.readErrMu.Unlock()
		close(c.done)
	})
}

func (c *i2pDatagramPeerConn) getReadErr() error {
	c.readErrMu.RLock()
	defer c.readErrMu.RUnlock()
	return c.readErr
}

func (c *i2pDatagramPeerConn) getReadDeadline() time.Time {
	c.deadlineMu.RLock()
	defer c.deadlineMu.RUnlock()
	return c.readDeadline
}

func (c *i2pDatagramPeerConn) getWriteDeadline() time.Time {
	c.deadlineMu.RLock()
	defer c.deadlineMu.RUnlock()
	return c.writeDeadline
}

func (c *i2pDatagramPeerConn) deadlineTimer(deadline time.Time) (*time.Timer, <-chan time.Time, error) {
	if deadline.IsZero() {
		return nil, nil, nil
	}
	if !time.Now().Before(deadline) {
		return nil, nil, os.ErrDeadlineExceeded
	}
	timer := time.NewTimer(time.Until(deadline))
	return timer, timer.C, nil
}

type i2pDatagramAddr struct {
	destination string
}

func (a i2pDatagramAddr) Network() string {
	return anonymousI2PDatagramNetwork
}

func (a i2pDatagramAddr) String() string {
	return anonymousI2PDatagramNetwork + ":" + shortI2PDestination(a.destination)
}

func shortI2PDestination(destination string) string {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(destination))
	return hex.EncodeToString(sum[:8])
}
