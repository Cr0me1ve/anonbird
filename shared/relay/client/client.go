package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	auth "github.com/netbirdio/netbird/shared/relay/auth/hmac"
	"github.com/netbirdio/netbird/shared/relay/client/dialer"
	"github.com/netbirdio/netbird/shared/relay/healthcheck"
	"github.com/netbirdio/netbird/shared/relay/messages"
)

const (
	bufferSize            = 8820
	serverResponseTimeout = 8 * time.Second
	connChannelSize       = 100
)

var (
	ErrConnAlreadyExists = fmt.Errorf("connection already exists")
)

type internalStopFlag struct {
	sync.Mutex
	stop bool
}

func newInternalStopFlag() *internalStopFlag {
	return &internalStopFlag{}
}

func (isf *internalStopFlag) set() {
	isf.Lock()
	defer isf.Unlock()
	isf.stop = true
}

func (isf *internalStopFlag) isSet() bool {
	isf.Lock()
	defer isf.Unlock()
	return isf.stop
}

// Msg carry the payload from the server to the client. With this struct, the net.Conn can free the buffer.
type Msg struct {
	Payload []byte

	bufPool *sync.Pool
	bufPtr  *[]byte
}

func (m *Msg) Free() {
	m.bufPool.Put(m.bufPtr)
}

// connContainer is a container for the connection to the peer. It is responsible for managing the messages from the
// server and forwarding them to the upper layer content reader.
type connContainer struct {
	log         *log.Entry
	conn        *Conn
	messages    chan Msg
	msgChanLock sync.Mutex
	closed      bool // flag to check if channel is closed
	ctx         context.Context
	cancel      context.CancelFunc
}

type connKey struct {
	peerID    messages.PeerID
	channelID uint32
}

func newConnContainer(log *log.Entry, c *Client, key connKey, instanceURL *RelayAddr) *connContainer {
	ctx, cancel := context.WithCancel(context.Background())
	msgChan := make(chan Msg, connChannelSize)
	cn := &Conn{
		dstID:       key.peerID,
		channelID:   key.channelID,
		messageChan: msgChan,
		instanceURL: instanceURL,
	}
	cc := &connContainer{
		log:      log,
		conn:     cn,
		messages: msgChan,
		ctx:      ctx,
		cancel:   cancel,
	}

	// bind conn to client
	cn.writeFn = func(dstID messages.PeerID, channelID uint32, payload []byte) (int, error) {
		return c.writeTo(cc, dstID, channelID, payload)
	}
	cn.closeFn = func(dstID messages.PeerID, channelID uint32) error {
		return c.closeConn(cc, dstID, channelID)
	}
	cn.localAddrFn = func() net.Addr {
		return c.relayConn.LocalAddr()
	}
	return cc
}

func (cc *connContainer) netConn() net.Conn {
	return cc.conn
}

func (cc *connContainer) writeMsg(msg Msg) {
	cc.msgChanLock.Lock()
	defer cc.msgChanLock.Unlock()

	if cc.closed {
		msg.Free()
		return
	}

	select {
	case cc.messages <- msg:
	case <-cc.ctx.Done():
		msg.Free()
	default:
		msg.Free()
	}
}

func (cc *connContainer) close() {
	cc.cancel()

	cc.msgChanLock.Lock()
	defer cc.msgChanLock.Unlock()

	if cc.closed {
		return
	}

	cc.closed = true
	close(cc.messages)

	for msg := range cc.messages {
		msg.Free()
	}
}

// Client is a client for the relay server. It is responsible for establishing a connection to the relay server and
// managing connections to other peers. All exported functions are safe to call concurrently. After close the connection,
// the client can be reused by calling Connect again. When the client is closed, all connections are closed too.
// While the Connect is in progress, the OpenConn function will block until the connection is established with relay server.
type Client struct {
	log            *log.Entry
	connectionURL  string
	serverIP       netip.Addr
	authTokenStore *auth.TokenStore
	hashedID       messages.PeerID

	bufPool *sync.Pool

	relayConn        net.Conn
	conns            map[connKey]*connContainer
	peerConnRefs     map[messages.PeerID]int
	earlyMsgs        *earlyMsgBuffer
	serviceIsRunning bool
	mu               sync.Mutex // protect serviceIsRunning and conns
	readLoopMutex    sync.Mutex
	wgReadLoop       sync.WaitGroup
	instanceURL      *RelayAddr
	muInstanceURL    sync.Mutex

	onDisconnectListener func(string)
	listenerMutex        sync.Mutex

	stateSubscription *PeersStateSubscription

	mtu uint16

	socks5Proxy       string
	i2pSAM            string
	i2pTunnelLength   uint8
	i2pTunnelQuantity uint8
	relayChannelID    uint32
}

// NewClient creates a new client for the relay server. The client is not connected to the server until the Connect
// is called.
func NewClient(serverURL string, authTokenStore *auth.TokenStore, peerID string, mtu uint16) *Client {
	return NewClientWithServerIP(serverURL, netip.Addr{}, authTokenStore, peerID, mtu)
}

func NewClientWithSOCKS5(serverURL string, authTokenStore *auth.TokenStore, peerID string, mtu uint16, socks5Proxy string) *Client {
	return newClient(serverURL, netip.Addr{}, authTokenStore, peerID, mtu, socks5Proxy, "", 0, 0)
}

func NewClientWithI2P(serverURL string, authTokenStore *auth.TokenStore, peerID string, mtu uint16, i2pSAM string, tunnelLength, tunnelQuantity uint8) *Client {
	return newClient(serverURL, netip.Addr{}, authTokenStore, peerID, mtu, "", i2pSAM, tunnelLength, tunnelQuantity)
}

// NewClientWithServerIP creates a new client for the relay server with a known server IP. serverIP, when valid, is
// dialed directly first; the FQDN is only attempted if the IP-based dial fails. TLS verification still uses the
// FQDN from serverURL via SNI.
func NewClientWithServerIP(serverURL string, serverIP netip.Addr, authTokenStore *auth.TokenStore, peerID string, mtu uint16) *Client {
	return newClient(serverURL, serverIP, authTokenStore, peerID, mtu, "", "", 0, 0)
}

func NewClientWithServerIPAndSOCKS5(serverURL string, serverIP netip.Addr, authTokenStore *auth.TokenStore, peerID string, mtu uint16, socks5Proxy string) *Client {
	return newClient(serverURL, serverIP, authTokenStore, peerID, mtu, socks5Proxy, "", 0, 0)
}

func NewClientWithServerIPAndI2P(serverURL string, serverIP netip.Addr, authTokenStore *auth.TokenStore, peerID string, mtu uint16, i2pSAM string, tunnelLength, tunnelQuantity uint8) *Client {
	return newClient(serverURL, serverIP, authTokenStore, peerID, mtu, "", i2pSAM, tunnelLength, tunnelQuantity)
}

func newClient(serverURL string, serverIP netip.Addr, authTokenStore *auth.TokenStore, peerID string, mtu uint16, socks5Proxy string, i2pSAM string, i2pTunnelLength, i2pTunnelQuantity uint8) *Client {
	return newClientWithRelayChannel(serverURL, serverIP, authTokenStore, peerID, mtu, socks5Proxy, i2pSAM, i2pTunnelLength, i2pTunnelQuantity, 0)
}

func newClientWithRelayChannel(serverURL string, serverIP netip.Addr, authTokenStore *auth.TokenStore, peerID string, mtu uint16, socks5Proxy string, i2pSAM string, i2pTunnelLength, i2pTunnelQuantity uint8, relayChannelID uint32) *Client {
	hashedID := messages.HashID(peerID)
	relayLog := log.WithFields(log.Fields{"relay": serverURL})
	if relayChannelID != 0 {
		relayLog = relayLog.WithField("relay_channel", relayChannelID)
	}

	c := &Client{
		log:               relayLog,
		connectionURL:     serverURL,
		serverIP:          serverIP,
		authTokenStore:    authTokenStore,
		hashedID:          hashedID,
		mtu:               mtu,
		socks5Proxy:       socks5Proxy,
		i2pSAM:            i2pSAM,
		i2pTunnelLength:   i2pTunnelLength,
		i2pTunnelQuantity: i2pTunnelQuantity,
		relayChannelID:    relayChannelID,
		bufPool: &sync.Pool{
			New: func() any {
				buf := make([]byte, bufferSize)
				return &buf
			},
		},
		conns:        make(map[connKey]*connContainer),
		peerConnRefs: make(map[messages.PeerID]int),
	}

	c.earlyMsgs = newEarlyMsgBuffer()

	c.log.Infof("create new relay connection: local peerID: %s, local peer hashedID: %s", peerID, hashedID)
	return c
}

// Connect establishes a connection to the relay server. It blocks until the connection is established or an error occurs.
func (c *Client) Connect(ctx context.Context) error {
	c.log.Infof("connecting to relay server")
	c.readLoopMutex.Lock()
	defer c.readLoopMutex.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.serviceIsRunning {
		return nil
	}

	instanceURL, err := c.connect(ctx)
	if err != nil {
		return err
	}
	c.muInstanceURL.Lock()
	c.instanceURL = instanceURL
	c.muInstanceURL.Unlock()

	c.stateSubscription = NewPeersStateSubscription(c.log, c.relayConn, c.closeConnsByPeerID)

	c.log = c.log.WithField("relay", instanceURL.String())
	c.log.Infof("relay connection established")

	c.serviceIsRunning = true

	internallyStoppedFlag := newInternalStopFlag()
	hc := c.newHealthcheckReceiver()
	go c.listenForStopEvents(ctx, hc, c.relayConn, internallyStoppedFlag)

	c.wgReadLoop.Add(1)
	go c.readLoop(hc, c.relayConn, internallyStoppedFlag)

	return nil
}

func (c *Client) newHealthcheckReceiver() *healthcheck.Receiver {
	if c.usesAnonymousRelayTransport() {
		return healthcheck.NewAnonymousReceiver(c.log)
	}
	return healthcheck.NewReceiver(c.log)
}

func (c *Client) usesAnonymousRelayTransport() bool {
	return c.socks5Proxy != "" || c.i2pSAM != ""
}

// OpenConn create a new net.Conn for the destination peer ID. In case if the connection is in progress
// to the relay server, the function will block until the connection is established or timed out. Otherwise,
// it will return immediately.
// It block until the server confirm the peer is online.
// todo: what should happen if call with the same peerID with multiple times?
func (c *Client) OpenConn(ctx context.Context, dstPeerID string) (net.Conn, error) {
	return c.OpenConnChannel(ctx, dstPeerID, 0)
}

// OpenConnChannel creates a logical relayed connection to dstPeerID on the
// given channel. Channel 0 is the legacy/default relay channel used by OpenConn.
func (c *Client) OpenConnChannel(ctx context.Context, dstPeerID string, channelID uint32) (net.Conn, error) {
	peerID := messages.HashID(dstPeerID)
	key := connKey{peerID: peerID, channelID: channelID}

	c.mu.Lock()
	if !c.serviceIsRunning {
		c.mu.Unlock()
		return nil, fmt.Errorf("relay connection is not established")
	}
	_, ok := c.conns[key]
	if ok {
		c.mu.Unlock()
		return nil, ErrConnAlreadyExists
	}
	alreadySubscribed := c.peerConnRefs[peerID] > 0

	c.log.Infof("prepare the relayed connection, waiting for remote peer: %s channel: %d", peerID, channelID)

	c.muInstanceURL.Lock()
	instanceURL := c.instanceURL
	c.muInstanceURL.Unlock()

	container := newConnContainer(c.log, c, key, instanceURL)
	c.conns[key] = container
	earlyMsg, hasEarly := c.earlyMsgs.popKey(key)
	c.mu.Unlock()

	if hasEarly {
		container.writeMsg(earlyMsg)
		c.log.Tracef("flushed buffered early message for peer: %s", peerID)
	}

	if !alreadySubscribed {
		if err := c.stateSubscription.WaitToBeOnlineAndSubscribe(ctx, peerID); err != nil {
			c.log.Errorf("peer not available: %s, %s", peerID, err)
			c.mu.Lock()
			if savedContainer, ok := c.conns[key]; ok && savedContainer == container {
				delete(c.conns, key)
			}
			c.mu.Unlock()
			container.close()
			return nil, err
		}
	}

	c.mu.Lock()
	if savedContainer, ok := c.conns[key]; ok && savedContainer == container {
		c.peerConnRefs[peerID]++
	}
	c.mu.Unlock()

	c.mu.Lock()
	if !c.serviceIsRunning {
		if savedContainer, ok := c.conns[key]; ok && savedContainer == container {
			delete(c.conns, key)
			if c.peerConnRefs[peerID] > 0 {
				c.peerConnRefs[peerID]--
				if c.peerConnRefs[peerID] == 0 {
					delete(c.peerConnRefs, peerID)
				}
			}
		}
		c.mu.Unlock()
		container.close()
		return nil, fmt.Errorf("relay connection is not established")
	}
	c.mu.Unlock()

	c.log.Infof("remote peer is available: %s channel: %d", peerID, channelID)
	return container.netConn(), nil
}

// ServerInstanceURL returns the address of the relay server. It could change after the close and reopen the connection.
func (c *Client) ServerInstanceURL() (string, error) {
	c.muInstanceURL.Lock()
	defer c.muInstanceURL.Unlock()
	if c.instanceURL == nil {
		return "", fmt.Errorf("relay connection is not established")
	}
	return c.instanceURL.String(), nil
}

// ConnectedIP returns the IP address of the live relay-server connection,
// extracted from the underlying socket's RemoteAddr. Zero value if not
// connected or if the address is not an IP literal.
func (c *Client) ConnectedIP() netip.Addr {
	c.mu.Lock()
	conn := c.relayConn
	c.mu.Unlock()
	if conn == nil {
		return netip.Addr{}
	}
	addr := conn.RemoteAddr()
	if addr == nil {
		return netip.Addr{}
	}
	return extractIPLiteral(addr.String())
}

// SetOnDisconnectListener sets a function that will be called when the connection to the relay server is closed.
func (c *Client) SetOnDisconnectListener(fn func(string)) {
	c.listenerMutex.Lock()
	defer c.listenerMutex.Unlock()
	c.onDisconnectListener = fn
}

// HasConns returns true if there are connections.
func (c *Client) HasConns() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.conns) > 0
}

func (c *Client) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serviceIsRunning
}

// Close closes the connection to the relay server and all connections to other peers.
func (c *Client) Close() error {
	return c.close(true)
}

func (c *Client) connect(ctx context.Context) (*RelayAddr, error) {
	dialers := c.getDialers()

	var conn net.Conn
	if c.shouldDialServerIP() {
		var err error
		conn, err = c.dialRaceDirect(ctx, dialers)
		if err != nil {
			c.log.Infof("dial via server IP %s failed, falling back to FQDN: %v", c.serverIP, err)
			conn = nil
		}
	}

	if conn == nil {
		rd := dialer.NewRaceDial(c.log, dialer.DefaultConnectionTimeout, c.connectionURL, dialers...)
		var err error
		conn, err = rd.Dial(ctx)
		if err != nil {
			return nil, fmt.Errorf("dial via FQDN: %w", err)
		}
	}
	c.relayConn = conn

	instanceURL, err := c.handShake(ctx)
	if err != nil {
		cErr := conn.Close()
		if cErr != nil {
			c.log.Errorf("failed to close connection: %s", cErr)
		}
		return nil, err
	}

	return instanceURL, nil
}

func (c *Client) shouldDialServerIP() bool {
	return c.serverIP.IsValid() && c.socks5Proxy == "" && c.i2pSAM == ""
}

// dialRaceDirect dials c.serverIP, preserving the original FQDN as the TLS ServerName for SNI.
func (c *Client) dialRaceDirect(ctx context.Context, dialers []dialer.DialeFn) (net.Conn, error) {
	directURL, serverName, err := substituteHost(c.connectionURL, c.serverIP)
	if err != nil {
		return nil, fmt.Errorf("substitute host: %w", err)
	}

	c.log.Debugf("dialing via server IP %s (SNI=%s)", c.serverIP, serverName)

	rd := dialer.NewRaceDial(c.log, dialer.DefaultConnectionTimeout, directURL, dialers...).
		WithServerName(serverName)
	return rd.Dial(ctx)
}

// substituteHost replaces the host portion of a rel/rels URL with ip,
// preserving the scheme and port. Returns the rewritten URL and the
// original host to use as the TLS ServerName, or empty if the original
// host is itself an IP literal (SNI requires a DNS name).
func substituteHost(serverURL string, ip netip.Addr) (string, string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", "", fmt.Errorf("parse %q: %w", serverURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid relay URL %q", serverURL)
	}
	if !ip.IsValid() {
		return "", "", errors.New("invalid server IP")
	}
	origHost := u.Hostname()
	if _, err := netip.ParseAddr(origHost); err == nil {
		origHost = ""
	}
	ip = ip.Unmap()
	newHost := ip.String()
	if ip.Is6() {
		newHost = "[" + newHost + "]"
	}
	if port := u.Port(); port != "" {
		u.Host = newHost + ":" + port
	} else {
		u.Host = newHost
	}
	return u.String(), origHost, nil
}

func (c *Client) handShake(ctx context.Context) (*RelayAddr, error) {
	msg, err := messages.MarshalAuthChannelMsg(c.hashedID, c.relayChannelID, c.authTokenStore.TokenBinary())
	if err != nil {
		c.log.Errorf("failed to marshal auth message: %s", err)
		return nil, err
	}

	_, err = c.relayConn.Write(msg)
	if err != nil {
		c.log.Errorf("failed to send auth message: %s", err)
		return nil, err
	}
	buf := make([]byte, messages.MaxHandshakeRespSize)
	n, err := c.readWithTimeout(ctx, buf)
	if err != nil {
		c.log.Errorf("failed to read auth response: %s", err)
		return nil, err
	}

	_, err = messages.ValidateVersion(buf[:n])
	if err != nil {
		return nil, fmt.Errorf("validate version: %w", err)
	}

	msgType, err := messages.DetermineServerMessageType(buf[:n])
	if err != nil {
		c.log.Errorf("failed to determine message type: %s", err)
		return nil, err
	}

	if msgType != messages.MsgTypeAuthResponse {
		c.log.Errorf("unexpected message type: %s", msgType)
		return nil, fmt.Errorf("unexpected message type")
	}

	addr, err := messages.UnmarshalAuthResponse(buf[:n])
	if err != nil {
		return nil, err
	}

	return &RelayAddr{addr: addr}, nil
}

func (c *Client) readLoop(hc *healthcheck.Receiver, relayConn net.Conn, internallyStoppedFlag *internalStopFlag) {
	var (
		errExit error
		n       int
	)
	for {
		bufPtr := c.bufPool.Get().(*[]byte)
		buf := *bufPtr
		n, errExit = relayConn.Read(buf)
		if errExit != nil {
			c.log.Infof("start to Relay read loop exit")
			c.mu.Lock()
			if c.serviceIsRunning && !internallyStoppedFlag.isSet() {
				c.log.Errorf("failed to read message from relay server: %s", errExit)
			}
			c.mu.Unlock()
			c.bufPool.Put(bufPtr)
			break
		}

		buf = buf[:n]

		_, err := messages.ValidateVersion(buf)
		if err != nil {
			c.log.Errorf("failed to validate protocol version: %s", err)
			c.bufPool.Put(bufPtr)
			continue
		}

		msgType, err := messages.DetermineServerMessageType(buf)
		if err != nil {
			c.log.Errorf("failed to determine message type: %s", err)
			c.bufPool.Put(bufPtr)
			continue
		}

		if !c.handleMsg(msgType, buf, bufPtr, hc, internallyStoppedFlag) {
			break
		}
	}

	hc.Stop()

	c.stateSubscription.Cleanup()
	c.wgReadLoop.Done()
	_ = c.close(false)
	c.notifyDisconnected()
}

func (c *Client) handleMsg(msgType messages.MsgType, buf []byte, bufPtr *[]byte, hc *healthcheck.Receiver, internallyStoppedFlag *internalStopFlag) (continueLoop bool) {
	switch msgType {
	case messages.MsgTypeHealthCheck:
		c.handleHealthCheck(hc, internallyStoppedFlag)
		c.bufPool.Put(bufPtr)
	case messages.MsgTypeTransport, messages.MsgTypeTransportChannel:
		return c.handleTransportMsg(buf, bufPtr, internallyStoppedFlag)
	case messages.MsgTypePeersOnline:
		c.handlePeersOnlineMsg(buf)
		c.bufPool.Put(bufPtr)
		return true
	case messages.MsgTypePeersWentOffline:
		c.handlePeersWentOfflineMsg(buf)
		c.bufPool.Put(bufPtr)
		return true
	case messages.MsgTypeClose:
		c.log.Debugf("relay connection close by server")
		c.bufPool.Put(bufPtr)
		return false
	}

	return true
}

func (c *Client) handleHealthCheck(hc *healthcheck.Receiver, internallyStoppedFlag *internalStopFlag) {
	msg := messages.MarshalHealthcheck()
	_, wErr := c.relayConn.Write(msg)
	if wErr != nil {
		if c.serviceIsRunning && !internallyStoppedFlag.isSet() {
			c.log.Errorf("failed to send heartbeat: %s", wErr)
		}
	}
	hc.Heartbeat()
}

func (c *Client) handleTransportMsg(buf []byte, bufPtr *[]byte, internallyStoppedFlag *internalStopFlag) bool {
	peerID, channelID, payload, err := messages.UnmarshalTransportChannelMsg(buf)
	if err != nil {
		if c.serviceIsRunning && !internallyStoppedFlag.isSet() {
			c.log.Errorf("failed to parse transport message: %v", err)
		}

		c.bufPool.Put(bufPtr)
		return true
	}
	key := connKey{peerID: *peerID, channelID: channelID}

	c.mu.Lock()
	if !c.serviceIsRunning {
		c.mu.Unlock()
		c.bufPool.Put(bufPtr)
		return false
	}
	container, ok := c.conns[key]
	earlyBuf := c.earlyMsgs
	c.mu.Unlock()
	if !ok {
		msg := Msg{
			bufPool: c.bufPool,
			bufPtr:  bufPtr,
			Payload: payload,
		}
		if earlyBuf == nil || !earlyBuf.putKey(key, msg) {
			c.log.Warnf("failed to buffer early message for peer: %s channel: %d", peerID.String(), channelID)
			c.bufPool.Put(bufPtr)
		} else {
			c.log.Debugf("buffered early transport message for peer: %s channel: %d", peerID.String(), channelID)
		}
		return true
	}
	msg := Msg{
		bufPool: c.bufPool,
		bufPtr:  bufPtr,
		Payload: payload,
	}
	container.writeMsg(msg)
	return true
}

func (c *Client) writeTo(containerRef *connContainer, dstID messages.PeerID, channelID uint32, payload []byte) (int, error) {
	key := connKey{peerID: dstID, channelID: channelID}
	c.mu.Lock()
	current, ok := c.conns[key]
	c.mu.Unlock()
	if !ok {
		return 0, net.ErrClosed
	}

	if current != containerRef {
		return 0, net.ErrClosed
	}

	// todo: use buffer pool instead of create new transport msg.
	var (
		msg []byte
		err error
	)
	if channelID == 0 {
		msg, err = messages.MarshalTransportMsg(dstID, payload)
	} else {
		msg, err = messages.MarshalTransportChannelMsg(dstID, channelID, payload)
	}
	if err != nil {
		c.log.Errorf("failed to marshal transport message: %s", err)
		return 0, err
	}

	// the write always return with 0 length because the underling does not support the size feedback.
	_, err = c.relayConn.Write(msg)
	if err != nil {
		c.log.Errorf("failed to write transport message: %s", err)
	}
	return len(payload), err
}

func (c *Client) listenForStopEvents(ctx context.Context, hc *healthcheck.Receiver, conn net.Conn, internalStopFlag *internalStopFlag) {
	for {
		select {
		case _, ok := <-hc.OnTimeout:
			if !ok {
				return
			}
			c.log.Errorf("health check timeout")
			internalStopFlag.set()
			if err := conn.Close(); err != nil {
				// ignore the err handling because the readLoop will handle it
				c.log.Warnf("failed to close connection: %s", err)
			}
			return
		case <-ctx.Done():
			err := c.close(true)
			if err != nil {
				c.log.Errorf("failed to teardown connection: %s", err)
			}
			return
		}
	}
}

func (c *Client) closeAllConns() {
	for _, container := range c.conns {
		container.close()
	}
	c.conns = make(map[connKey]*connContainer)
	c.peerConnRefs = make(map[messages.PeerID]int)

	c.earlyMsgs.close()
	c.earlyMsgs = newEarlyMsgBuffer()
}

func (c *Client) closeConnsByPeerID(peerIDs []messages.PeerID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, peerID := range peerIDs {
		var found bool
		for key, container := range c.conns {
			if key.peerID != peerID {
				continue
			}
			found = true
			container.log.Infof("remote peer has been disconnected, free up connection: %s channel: %d", peerID, key.channelID)
			container.close()
			delete(c.conns, key)
		}
		delete(c.peerConnRefs, peerID)
		if !found {
			c.log.Warnf("can not close connection, peer not found: %s", peerID)
			continue
		}
	}

	if err := c.stateSubscription.UnsubscribeStateChange(peerIDs); err != nil {
		c.log.Errorf("failed to unsubscribe from peer state change: %s, %s", peerIDs, err)
	}
}

func (c *Client) closeConn(containerRef *connContainer, id messages.PeerID, channelID uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := connKey{peerID: id, channelID: channelID}
	current, ok := c.conns[key]
	if !ok {
		return net.ErrClosed
	}

	if current != containerRef {
		return fmt.Errorf("conn reference mismatch")
	}

	remainingRefs := c.peerConnRefs[id] - 1
	if remainingRefs <= 0 {
		if err := c.stateSubscription.UnsubscribeStateChange([]messages.PeerID{id}); err != nil {
			current.log.Errorf("failed to unsubscribe from peer state change: %s", err)
		}
		delete(c.peerConnRefs, id)
	} else {
		c.peerConnRefs[id] = remainingRefs
	}

	c.log.Infof("free up connection to peer: %s channel: %d", id, channelID)
	delete(c.conns, key)
	current.close()

	return nil
}

func (c *Client) close(gracefullyExit bool) error {
	c.readLoopMutex.Lock()
	defer c.readLoopMutex.Unlock()

	c.mu.Lock()
	var err error
	if !c.serviceIsRunning {
		c.mu.Unlock()
		c.log.Warn("relay connection was already marked as not running")
		return nil
	}
	c.serviceIsRunning = false

	c.muInstanceURL.Lock()
	c.instanceURL = nil
	c.muInstanceURL.Unlock()

	c.log.Infof("closing all peer connections")
	c.closeAllConns()
	if gracefullyExit {
		c.writeCloseMsg()
	}
	err = c.relayConn.Close()
	c.mu.Unlock()

	c.log.Infof("waiting for read loop to close")
	c.wgReadLoop.Wait()
	c.log.Infof("relay connection closed")
	return err
}

func (c *Client) notifyDisconnected() {
	c.listenerMutex.Lock()
	defer c.listenerMutex.Unlock()

	if c.onDisconnectListener == nil {
		return
	}
	go c.onDisconnectListener(c.connectionURL)
}

func (c *Client) writeCloseMsg() {
	msg := messages.MarshalCloseMsg()
	_, err := c.relayConn.Write(msg)
	if err != nil {
		c.log.Errorf("failed to send close message: %s", err)
	}
}

func (c *Client) readWithTimeout(ctx context.Context, buf []byte) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, serverResponseTimeout)
	defer cancel()

	readDone := make(chan struct{})
	var (
		n   int
		err error
	)

	go func() {
		n, err = c.relayConn.Read(buf)
		close(readDone)
	}()

	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("read operation timed out")
	case <-readDone:
		return n, err
	}
}

func (c *Client) handlePeersOnlineMsg(buf []byte) {
	peersID, err := messages.UnmarshalPeersOnlineMsg(buf)
	if err != nil {
		c.log.Errorf("failed to unmarshal peers online msg: %s", err)
		return
	}
	c.stateSubscription.OnPeersOnline(peersID)
}

func (c *Client) handlePeersWentOfflineMsg(buf []byte) {
	peersID, err := messages.UnMarshalPeersWentOffline(buf)
	if err != nil {
		c.log.Errorf("failed to unmarshal peers went offline msg: %s", err)
		return
	}
	c.stateSubscription.OnPeersWentOffline(peersID)
}

// extractIPLiteral returns the IP from address forms produced by the relay
// dialers (URL or host:port). Zero value if the host is not an IP.
func extractIPLiteral(s string) netip.Addr {
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Host
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		host = s
	}
	host = strings.Trim(host, "[]")
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}
