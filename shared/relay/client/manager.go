package client

import (
	"container/list"
	"context"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	relayAuth "github.com/netbirdio/netbird/shared/relay/auth/hmac"
)

var (
	relayCleanupInterval = 60 * time.Second
	keepUnusedServerTime = 5 * time.Second

	ErrRelayClientNotConnected = fmt.Errorf("relay client not connected")
)

// RelayTrack hold the relay clients for the foreign relay servers.
// With the mutex can ensure we can open new connection in case the relay connection has been established with
// the relay server.
type RelayTrack struct {
	sync.RWMutex
	relayClient *Client
	err         error
	created     time.Time
}

type dedicatedRelayKey struct {
	serverAddress string
	channelID     uint32
}

func NewRelayTrack() *RelayTrack {
	return &RelayTrack{
		created: time.Now(),
	}
}

type OnServerCloseListener func()

// ManagerOption configures a Manager at construction time.
type ManagerOption func(*Manager)

// WithMaxBackoffInterval caps the exponential backoff between reconnect
// attempts to the home relay. A non-positive value keeps the default.
func WithMaxBackoffInterval(d time.Duration) ManagerOption {
	return func(m *Manager) { m.maxBackoffInterval = d }
}

func WithSOCKS5Proxy(socks5Proxy string) ManagerOption {
	return func(m *Manager) {
		m.socks5Proxy = socks5Proxy
		if m.serverPicker != nil {
			m.serverPicker.SOCKS5Proxy = socks5Proxy
		}
	}
}

func WithI2PSAM(i2pSAM string, tunnelLength, tunnelQuantity uint8) ManagerOption {
	return func(m *Manager) {
		m.i2pSAM = i2pSAM
		m.i2pTunnelLength = tunnelLength
		m.i2pTunnelQuantity = tunnelQuantity
		if m.serverPicker != nil {
			m.serverPicker.I2PSAM = i2pSAM
			m.serverPicker.I2PTunnelLength = tunnelLength
			m.serverPicker.I2PTunnelQuantity = tunnelQuantity
		}
	}
}

// Manager is a manager for the relay client instances. It establishes one persistent connection to the given relay URL
// and automatically reconnect to them in case disconnection.
// The manager also manage temporary relay connection. If a client wants to communicate with a client on a
// different relay server, the manager will establish a new connection to the relay server. The connection with these
// relay servers will be closed if there is no active connection. Periodically the manager will check if there is any
// unused relay connection and close it.
type Manager struct {
	ctx          context.Context
	peerID       string
	running      bool
	tokenStore   *relayAuth.TokenStore
	serverPicker *ServerPicker

	relayClient *Client
	// the guard logic can overwrite the relayClient variable, this mutex protect the usage of the variable
	relayClientMu  sync.RWMutex
	reconnectGuard *Guard

	relayClients      map[string]*RelayTrack
	relayClientsMutex sync.RWMutex

	dedicatedRelayClients      map[dedicatedRelayKey]*RelayTrack
	dedicatedRelayClientsMutex sync.RWMutex

	onDisconnectedListeners map[string]*list.List
	onReconnectedListenerFn func()
	listenerLock            sync.Mutex

	mtu                uint16
	maxBackoffInterval time.Duration
	socks5Proxy        string
	i2pSAM             string
	i2pTunnelLength    uint8
	i2pTunnelQuantity  uint8

	cleanupInterval      time.Duration
	keepUnusedServerTime time.Duration
}

// NewManager creates a new manager instance.
// The serverURL address can be empty. In this case, the manager will not serve.
func NewManager(ctx context.Context, serverURLs []string, peerID string, mtu uint16, opts ...ManagerOption) *Manager {
	tokenStore := &relayAuth.TokenStore{}

	m := &Manager{
		ctx:        ctx,
		peerID:     peerID,
		tokenStore: tokenStore,
		mtu:        mtu,
		serverPicker: &ServerPicker{
			TokenStore:        tokenStore,
			PeerID:            peerID,
			MTU:               mtu,
			ConnectionTimeout: defaultConnectionTimeout,
		},
		relayClients:            make(map[string]*RelayTrack),
		dedicatedRelayClients:   make(map[dedicatedRelayKey]*RelayTrack),
		onDisconnectedListeners: make(map[string]*list.List),
		cleanupInterval:         relayCleanupInterval,
		keepUnusedServerTime:    keepUnusedServerTime,
	}
	for _, opt := range opts {
		opt(m)
	}
	m.serverPicker.SOCKS5Proxy = m.socks5Proxy
	m.serverPicker.I2PSAM = m.i2pSAM
	m.serverPicker.I2PTunnelLength = m.i2pTunnelLength
	m.serverPicker.I2PTunnelQuantity = m.i2pTunnelQuantity
	m.serverPicker.ServerURLs.Store(serverURLs)
	m.reconnectGuard = NewGuard(m.serverPicker, m.maxBackoffInterval)
	return m
}

// Serve starts the manager, attempting to establish a connection with the relay server.
// If the connection fails, it will keep trying to reconnect in the background.
// Additionally, it starts a cleanup loop to remove unused relay connections.
// The manager will automatically reconnect to the relay server in case of disconnection.
func (m *Manager) Serve() error {
	if m.running {
		return fmt.Errorf("manager already serving")
	}
	m.running = true
	log.Debugf("starting relay client manager with %v relay servers", m.serverPicker.ServerURLs.Load())

	client, err := m.serverPicker.PickServer(m.ctx)
	if err != nil {
		go m.reconnectGuard.StartReconnectTrys(m.ctx, nil)
	} else {
		m.storeClient(client)
	}

	go m.listenGuardEvent(m.ctx)
	go m.startCleanupLoop()
	return err
}

// OpenConn opens a connection to the given peer key. If the peer is on the same relay server, the connection will be
// established via the relay server. If the peer is on a different relay server, the manager will establish a new
// connection to the relay server. It returns back with a net.Conn what represent the remote peer connection.
//
// serverIP, when valid and serverAddress is foreign, is used as a dial target if the FQDN-based dial fails.
// Ignored for the local home-server path. TLS verification still uses the FQDN via SNI.
func (m *Manager) OpenConn(ctx context.Context, serverAddress, peerKey string, serverIP netip.Addr) (net.Conn, error) {
	return m.OpenConnChannel(ctx, serverAddress, peerKey, serverIP, 0)
}

// OpenConnChannel opens a logical relay connection on a specific transport channel.
// Channel 0 is the legacy/default relay channel used by OpenConn.
func (m *Manager) OpenConnChannel(ctx context.Context, serverAddress, peerKey string, serverIP netip.Addr, channelID uint32) (net.Conn, error) {
	m.relayClientMu.RLock()
	defer m.relayClientMu.RUnlock()

	if m.relayClient == nil {
		return nil, ErrRelayClientNotConnected
	}

	foreign, err := m.isForeignServer(serverAddress)
	if err != nil {
		return nil, err
	}

	var (
		netConn net.Conn
	)
	if !foreign {
		log.Debugf("open peer connection via permanent server: %s", peerKey)
		if m.useDedicatedAnonymousRelayChannel(channelID) {
			netConn, err = m.openDedicatedConn(ctx, serverAddress, peerKey, serverIP, channelID)
		} else {
			netConn, err = m.relayClient.OpenConnChannel(ctx, peerKey, channelID)
		}
	} else {
		log.Debugf("open peer connection via foreign server: %s", serverAddress)
		if m.useDedicatedAnonymousRelayChannel(channelID) {
			netConn, err = m.openDedicatedConn(ctx, serverAddress, peerKey, serverIP, channelID)
		} else {
			netConn, err = m.openConnVia(ctx, serverAddress, peerKey, serverIP, channelID)
		}
	}
	if err != nil {
		return nil, err
	}

	return netConn, err
}

func (m *Manager) useDedicatedAnonymousRelayChannel(channelID uint32) bool {
	return channelID != 0 && (m.socks5Proxy != "" || m.i2pSAM != "")
}

// Ready returns true if the home Relay client is connected to the relay server.
func (m *Manager) Ready() bool {
	m.relayClientMu.RLock()
	defer m.relayClientMu.RUnlock()

	if m.relayClient == nil {
		return false
	}
	return m.relayClient.Ready()
}

func (m *Manager) SetOnReconnectedListener(f func()) {
	m.listenerLock.Lock()
	defer m.listenerLock.Unlock()

	m.onReconnectedListenerFn = f
}

// AddCloseListener adds a listener to the given server instance address. The listener will be called if the connection
// closed.
func (m *Manager) AddCloseListener(serverAddress string, onClosedListener OnServerCloseListener) error {
	m.relayClientMu.RLock()
	defer m.relayClientMu.RUnlock()

	if m.relayClient == nil {
		return ErrRelayClientNotConnected
	}

	foreign, err := m.isForeignServer(serverAddress)
	if err != nil {
		return err
	}

	var listenerAddr string
	if foreign {
		listenerAddr = serverAddress
	} else {
		listenerAddr = m.relayClient.connectionURL
	}
	m.addListener(listenerAddr, onClosedListener)
	return nil
}

// RelayInstanceAddress returns the address and resolved IP of the permanent relay server. It could change if the
// network connection is lost. The address is sent to the target peer to choose the common relay server for the
// communication; the IP is sent alongside so remote peers can dial directly without their own DNS lookup. Both
// values are read under the same lock so they cannot diverge across a reconnection.
func (m *Manager) RelayInstanceAddress() (string, netip.Addr, error) {
	m.relayClientMu.RLock()
	defer m.relayClientMu.RUnlock()

	if m.relayClient == nil {
		return "", netip.Addr{}, ErrRelayClientNotConnected
	}
	addr, err := m.relayClient.ServerInstanceURL()
	if err != nil {
		return "", netip.Addr{}, err
	}
	if m.socks5Proxy != "" || m.i2pSAM != "" {
		return addr, netip.Addr{}, nil
	}
	return addr, m.relayClient.ConnectedIP(), nil
}

// ServerURLs returns the addresses of the relay servers.
func (m *Manager) ServerURLs() []string {
	return m.serverPicker.ServerURLs.Load().([]string)
}

// HasRelayAddress returns true if the manager is serving. With this method can check if the peer can communicate with
// Relay service.
func (m *Manager) HasRelayAddress() bool {
	return len(m.serverPicker.ServerURLs.Load().([]string)) > 0
}

func (m *Manager) UpdateServerURLs(serverURLs []string) {
	log.Infof("update relay server URLs: %v", serverURLs)
	m.serverPicker.ServerURLs.Store(serverURLs)
}

// UpdateToken updates the token in the token store.
func (m *Manager) UpdateToken(token *relayAuth.Token) error {
	return m.tokenStore.UpdateToken(token)
}

func (m *Manager) openConnVia(ctx context.Context, serverAddress, peerKey string, serverIP netip.Addr, channelID uint32) (net.Conn, error) {
	// check if already has a connection to the desired relay server
	m.relayClientsMutex.RLock()
	rt, ok := m.relayClients[serverAddress]
	if ok {
		rt.RLock()
		m.relayClientsMutex.RUnlock()
		defer rt.RUnlock()
		if rt.err != nil {
			return nil, rt.err
		}
		return rt.relayClient.OpenConnChannel(ctx, peerKey, channelID)
	}
	m.relayClientsMutex.RUnlock()

	// if not, establish a new connection but check it again (because changed the lock type) before starting the
	// connection
	m.relayClientsMutex.Lock()
	rt, ok = m.relayClients[serverAddress]
	if ok {
		rt.RLock()
		m.relayClientsMutex.Unlock()
		defer rt.RUnlock()
		if rt.err != nil {
			return nil, rt.err
		}
		return rt.relayClient.OpenConnChannel(ctx, peerKey, channelID)
	}

	// create a new relay client and store it in the relayClients map
	rt = NewRelayTrack()
	rt.Lock()
	m.relayClients[serverAddress] = rt
	m.relayClientsMutex.Unlock()

	if m.socks5Proxy != "" || m.i2pSAM != "" {
		serverIP = netip.Addr{}
	}
	relayClient := NewClientWithServerIPAndSOCKS5(serverAddress, serverIP, m.tokenStore, m.peerID, m.mtu, m.socks5Proxy)
	if m.i2pSAM != "" {
		relayClient = NewClientWithServerIPAndI2P(serverAddress, serverIP, m.tokenStore, m.peerID, m.mtu, m.i2pSAM, m.i2pTunnelLength, m.i2pTunnelQuantity)
	}
	err := relayClient.Connect(m.ctx)
	if err != nil {
		rt.err = err
		rt.Unlock()
		m.relayClientsMutex.Lock()
		delete(m.relayClients, serverAddress)
		m.relayClientsMutex.Unlock()
		return nil, err
	}
	// if connection closed then delete the relay client from the list
	relayClient.SetOnDisconnectListener(m.onServerDisconnected)
	rt.relayClient = relayClient
	rt.Unlock()

	conn, err := relayClient.OpenConnChannel(ctx, peerKey, channelID)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (m *Manager) openDedicatedConn(ctx context.Context, serverAddress, peerKey string, serverIP netip.Addr, channelID uint32) (net.Conn, error) {
	if m.socks5Proxy != "" || m.i2pSAM != "" {
		serverIP = netip.Addr{}
	}

	key := dedicatedRelayKey{serverAddress: serverAddress, channelID: channelID}
	relayClient, err := m.dedicatedRelayClient(ctx, key, serverIP)
	if err != nil {
		return nil, err
	}
	return relayClient.openConnChannelAssumePeerOnline(ctx, peerKey, channelID)
}

func (m *Manager) dedicatedRelayClient(ctx context.Context, key dedicatedRelayKey, serverIP netip.Addr) (*Client, error) {
	for {
		if relayClient, ok, err := m.cachedDedicatedRelayClient(key); ok || err != nil {
			return relayClient, err
		}

		m.dedicatedRelayClientsMutex.Lock()
		if _, ok := m.dedicatedRelayClients[key]; ok {
			m.dedicatedRelayClientsMutex.Unlock()
			continue
		}

		rt := NewRelayTrack()
		rt.Lock()
		m.dedicatedRelayClients[key] = rt
		m.dedicatedRelayClientsMutex.Unlock()

		relayClient := m.newRelayClient(key.serverAddress, serverIP, key.channelID)
		if err := relayClient.Connect(ctx); err != nil {
			rt.err = err
			rt.Unlock()
			m.evictDedicatedRelay(key)
			return nil, err
		}
		relayClient.SetOnDisconnectListener(func(string) {
			m.evictDedicatedRelay(key)
		})
		rt.relayClient = relayClient
		rt.Unlock()
		return relayClient, nil
	}
}

func (m *Manager) cachedDedicatedRelayClient(key dedicatedRelayKey) (*Client, bool, error) {
	m.dedicatedRelayClientsMutex.RLock()
	rt, ok := m.dedicatedRelayClients[key]
	if !ok {
		m.dedicatedRelayClientsMutex.RUnlock()
		return nil, false, nil
	}

	rt.RLock()
	m.dedicatedRelayClientsMutex.RUnlock()
	err := rt.err
	relayClient := rt.relayClient
	rt.RUnlock()

	if err != nil {
		return nil, true, err
	}
	if relayClient == nil {
		return nil, true, ErrRelayClientNotConnected
	}
	if relayClient.Ready() {
		return relayClient, true, nil
	}

	log.Debugf("evicting not-ready dedicated relay client: %s channel %d", key.serverAddress, key.channelID)
	m.evictDedicatedRelay(key)
	return nil, false, nil
}

func (m *Manager) newRelayClient(serverAddress string, serverIP netip.Addr, channelID uint32) *Client {
	return newClientWithRelayChannel(
		serverAddress,
		serverIP,
		m.tokenStore,
		m.peerID,
		m.mtu,
		m.socks5Proxy,
		m.i2pSAM,
		m.i2pTunnelLength,
		m.i2pTunnelQuantity,
		channelID,
	)
}

func (m *Manager) onServerConnected() {
	m.listenerLock.Lock()
	defer m.listenerLock.Unlock()

	if m.onReconnectedListenerFn == nil {
		return
	}
	go m.onReconnectedListenerFn()
}

// onServerDisconnected handles relay disconnect events. For the home server it
// starts the reconnect guard. For foreign servers it evicts the now-dead client
// from the cache so the next OpenConn builds a fresh one instead of reusing a
// closed client.
func (m *Manager) onServerDisconnected(serverAddress string) {
	m.relayClientMu.Lock()
	isHome := m.relayClient != nil && serverAddress == m.relayClient.connectionURL
	if isHome {
		go func(client *Client) {
			m.reconnectGuard.StartReconnectTrys(m.ctx, client)
		}(m.relayClient)
	}
	m.relayClientMu.Unlock()

	if !isHome {
		m.evictForeignRelay(serverAddress)
	}

	m.notifyOnDisconnectListeners(serverAddress)
}

func (m *Manager) evictForeignRelay(serverAddress string) {
	m.relayClientsMutex.Lock()
	defer m.relayClientsMutex.Unlock()
	if _, ok := m.relayClients[serverAddress]; ok {
		delete(m.relayClients, serverAddress)
		log.Debugf("evicted disconnected foreign relay client: %s", serverAddress)
	}
}

func (m *Manager) evictDedicatedRelay(key dedicatedRelayKey) {
	m.dedicatedRelayClientsMutex.Lock()
	defer m.dedicatedRelayClientsMutex.Unlock()
	if _, ok := m.dedicatedRelayClients[key]; ok {
		delete(m.dedicatedRelayClients, key)
		log.Debugf("evicted disconnected dedicated relay client: %s channel %d", key.serverAddress, key.channelID)
	}
}

func (m *Manager) listenGuardEvent(ctx context.Context) {
	for {
		select {
		case <-m.reconnectGuard.OnReconnected:
			m.onServerConnected()
		case rc := <-m.reconnectGuard.OnNewRelayClient:
			m.storeClient(rc)
			m.onServerConnected()
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) storeClient(client *Client) {
	m.relayClientMu.Lock()
	defer m.relayClientMu.Unlock()

	m.relayClient = client
	m.relayClient.SetOnDisconnectListener(m.onServerDisconnected)
}

func (m *Manager) isForeignServer(address string) (bool, error) {
	rAddr, err := m.relayClient.ServerInstanceURL()
	if err != nil {
		return false, fmt.Errorf("relay client not connected")
	}
	return rAddr != address, nil
}

func (m *Manager) startCleanupLoop() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanUpUnusedRelays()
		}
	}
}

func (m *Manager) cleanUpUnusedRelays() {
	m.relayClientsMutex.Lock()
	for addr, rt := range m.relayClients {
		rt.Lock()
		// if the connection failed to the server the relay client will be nil
		// but the instance will be kept in the relayClients until the next locking
		if rt.err != nil {
			rt.Unlock()
			continue
		}

		if time.Since(rt.created) <= m.keepUnusedServerTime {
			rt.Unlock()
			continue
		}

		if rt.relayClient.HasConns() {
			rt.Unlock()
			continue
		}
		rt.relayClient.SetOnDisconnectListener(nil)
		go func(relayClient *Client) {
			_ = relayClient.Close()
		}(rt.relayClient)
		log.Debugf("clean up unused relay server connection: %s", addr)
		delete(m.relayClients, addr)
		rt.Unlock()
	}
	m.relayClientsMutex.Unlock()

	m.cleanUpUnusedDedicatedRelays()
}

func (m *Manager) cleanUpUnusedDedicatedRelays() {
	m.dedicatedRelayClientsMutex.Lock()
	defer m.dedicatedRelayClientsMutex.Unlock()

	for key, rt := range m.dedicatedRelayClients {
		rt.Lock()
		if rt.err != nil {
			rt.Unlock()
			continue
		}
		if time.Since(rt.created) <= m.keepUnusedServerTime {
			rt.Unlock()
			continue
		}
		if rt.relayClient.HasConns() {
			rt.Unlock()
			continue
		}
		rt.relayClient.SetOnDisconnectListener(nil)
		go func(relayClient *Client) {
			_ = relayClient.Close()
		}(rt.relayClient)
		log.Debugf("clean up unused dedicated relay connection: %s channel %d", key.serverAddress, key.channelID)
		delete(m.dedicatedRelayClients, key)
		rt.Unlock()
	}
}

func (m *Manager) addListener(serverAddress string, onClosedListener OnServerCloseListener) {
	m.listenerLock.Lock()
	defer m.listenerLock.Unlock()
	l, ok := m.onDisconnectedListeners[serverAddress]
	if !ok {
		l = list.New()
	}
	for e := l.Front(); e != nil; e = e.Next() {
		if reflect.ValueOf(e.Value).Pointer() == reflect.ValueOf(onClosedListener).Pointer() {
			return
		}
	}
	l.PushBack(onClosedListener)
	m.onDisconnectedListeners[serverAddress] = l
}

func (m *Manager) notifyOnDisconnectListeners(serverAddress string) {
	m.listenerLock.Lock()
	defer m.listenerLock.Unlock()

	l, ok := m.onDisconnectedListeners[serverAddress]
	if !ok {
		return
	}
	for e := l.Front(); e != nil; e = e.Next() {
		go e.Value.(OnServerCloseListener)()
	}
	delete(m.onDisconnectedListeners, serverAddress)
}
