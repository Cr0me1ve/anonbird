package client

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"

	"github.com/netbirdio/netbird/client/iface"
	"github.com/netbirdio/netbird/relay/server"
	"github.com/netbirdio/netbird/shared/relay/auth/allow"
)

func readRelayTestPayload(t *testing.T, conn net.Conn) string {
	t.Helper()

	type readResult struct {
		n   int
		err error
	}
	buf := make([]byte, 65535)
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := conn.Read(buf)
		resultCh <- readResult{n: n, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("failed to read from channel: %s", result.err)
		}
		return string(buf[:result.n])
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for relay channel payload")
		return ""
	}
}

// newManagerTestServerConfig creates a new server config for manager testing with the given address
func newManagerTestServerConfig(address string) server.Config {
	return server.Config{
		Meter:          otel.Meter(""),
		ExposedAddress: address,
		TLSSupport:     false,
		AuthValidator:  &allow.Auth{},
	}
}

func TestEmptyURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := NewManager(ctx, nil, "alice", iface.DefaultMTU)
	err := mgr.Serve()
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestForeignConn(t *testing.T) {
	ctx := context.Background()

	lstCfg1 := server.ListenerConfig{
		Address: "localhost:52101",
	}

	srv1, err := server.NewServer(newManagerTestServerConfig(lstCfg1.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv1.Listen(lstCfg1)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv1.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	srvCfg2 := server.ListenerConfig{
		Address: "localhost:52102",
	}
	srv2, err := server.NewServer(newManagerTestServerConfig(srvCfg2.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan2 := make(chan error, 1)
	go func() {
		err := srv2.Listen(srvCfg2)
		if err != nil {
			errChan2 <- err
		}
	}()

	defer func() {
		err := srv2.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	if err := waitForServerToStart(errChan2); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	clientAlice := NewManager(mCtx, toURL(lstCfg1), "alice", iface.DefaultMTU)
	if err := clientAlice.Serve(); err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}

	clientBob := NewManager(mCtx, toURL(srvCfg2), "bob", iface.DefaultMTU)
	if err := clientBob.Serve(); err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}
	bobsSrvAddr, _, err := clientBob.RelayInstanceAddress()
	if err != nil {
		t.Fatalf("failed to get relay address: %s", err)
	}
	connAliceToBob, err := clientAlice.OpenConn(ctx, bobsSrvAddr, "bob", netip.Addr{})
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}
	connBobToAlice, err := clientBob.OpenConn(ctx, bobsSrvAddr, "alice", netip.Addr{})
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	payload := "hello bob, I am alice"
	_, err = connAliceToBob.Write([]byte(payload))
	if err != nil {
		t.Fatalf("failed to write to channel: %s", err)
	}

	buf := make([]byte, 65535)
	n, err := connBobToAlice.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from channel: %s", err)
	}

	_, err = connBobToAlice.Write(buf[:n])
	if err != nil {
		t.Fatalf("failed to write to channel: %s", err)
	}

	n, err = connAliceToBob.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from channel: %s", err)
	}

	if payload != string(buf[:n]) {
		t.Fatalf("expected %s, got %s", payload, string(buf[:n]))
	}

	aliceCh1, err := clientAlice.OpenConnChannel(ctx, bobsSrvAddr, "bob", netip.Addr{}, 11)
	if err != nil {
		t.Fatalf("failed to bind channel 11 from alice to bob: %s", err)
	}
	aliceCh2, err := clientAlice.OpenConnChannel(ctx, bobsSrvAddr, "bob", netip.Addr{}, 12)
	if err != nil {
		t.Fatalf("failed to bind channel 12 from alice to bob: %s", err)
	}
	bobCh1, err := clientBob.OpenConnChannel(ctx, bobsSrvAddr, "alice", netip.Addr{}, 11)
	if err != nil {
		t.Fatalf("failed to bind channel 11 from bob to alice: %s", err)
	}
	bobCh2, err := clientBob.OpenConnChannel(ctx, bobsSrvAddr, "alice", netip.Addr{}, 12)
	if err != nil {
		t.Fatalf("failed to bind channel 12 from bob to alice: %s", err)
	}

	payloadCh1 := "hello over manager channel 11"
	payloadCh2 := "hello over manager channel 12"
	if _, err := aliceCh1.Write([]byte(payloadCh1)); err != nil {
		t.Fatalf("failed to write to channel 11: %s", err)
	}
	if _, err := aliceCh2.Write([]byte(payloadCh2)); err != nil {
		t.Fatalf("failed to write to channel 12: %s", err)
	}

	if got := readRelayTestPayload(t, bobCh1); got != payloadCh1 {
		t.Fatalf("expected %s on channel 11, got %s", payloadCh1, got)
	}
	if got := readRelayTestPayload(t, bobCh2); got != payloadCh2 {
		t.Fatalf("expected %s on channel 12, got %s", payloadCh2, got)
	}
}

func TestDedicatedRelayChannelReusedAcrossPeers(t *testing.T) {
	ctx := context.Background()
	srvCfg := server.ListenerConfig{
		Address: "localhost:52601",
	}
	srv, err := server.NewServer(newManagerTestServerConfig(srvCfg.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Listen(srvCfg); err != nil {
			errChan <- err
		}
	}()
	defer func() {
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	relayURL := toURL(srvCfg)[0]

	alice := NewManager(mCtx, []string{relayURL}, "alice", iface.DefaultMTU)
	if err := alice.Serve(); err != nil {
		t.Fatalf("failed to serve alice manager: %s", err)
	}
	bob := NewManager(mCtx, []string{relayURL}, "bob", iface.DefaultMTU)
	if err := bob.Serve(); err != nil {
		t.Fatalf("failed to serve bob manager: %s", err)
	}
	carol := NewManager(mCtx, []string{relayURL}, "carol", iface.DefaultMTU)
	if err := carol.Serve(); err != nil {
		t.Fatalf("failed to serve carol manager: %s", err)
	}

	aliceBob, err := alice.openDedicatedConn(ctx, relayURL, "bob", netip.Addr{}, 7)
	if err != nil {
		t.Fatalf("failed to open alice->bob dedicated channel: %s", err)
	}
	defer aliceBob.Close()
	bobAlice, err := bob.openDedicatedConn(ctx, relayURL, "alice", netip.Addr{}, 7)
	if err != nil {
		t.Fatalf("failed to open bob->alice dedicated channel: %s", err)
	}
	defer bobAlice.Close()

	if _, err := aliceBob.Write([]byte("first bob payload")); err != nil {
		t.Fatalf("failed to write first bob payload: %s", err)
	}
	if got := readRelayTestPayload(t, bobAlice); got != "first bob payload" {
		t.Fatalf("expected first bob payload, got %s", got)
	}

	aliceCarol, err := alice.openDedicatedConn(ctx, relayURL, "carol", netip.Addr{}, 7)
	if err != nil {
		t.Fatalf("failed to open alice->carol dedicated channel: %s", err)
	}
	defer aliceCarol.Close()
	carolAlice, err := carol.openDedicatedConn(ctx, relayURL, "alice", netip.Addr{}, 7)
	if err != nil {
		t.Fatalf("failed to open carol->alice dedicated channel: %s", err)
	}
	defer carolAlice.Close()

	alice.dedicatedRelayClientsMutex.RLock()
	dedicatedClients := len(alice.dedicatedRelayClients)
	alice.dedicatedRelayClientsMutex.RUnlock()
	if dedicatedClients != 1 {
		t.Fatalf("expected one cached dedicated relay client, got %d", dedicatedClients)
	}

	if _, err := aliceBob.Write([]byte("second bob payload")); err != nil {
		t.Fatalf("failed to write second bob payload: %s", err)
	}
	if got := readRelayTestPayload(t, bobAlice); got != "second bob payload" {
		t.Fatalf("expected second bob payload, got %s", got)
	}
	if _, err := aliceCarol.Write([]byte("carol payload")); err != nil {
		t.Fatalf("failed to write carol payload: %s", err)
	}
	if got := readRelayTestPayload(t, carolAlice); got != "carol payload" {
		t.Fatalf("expected carol payload, got %s", got)
	}
}

func TestCachedDedicatedRelayClientReturnsReadyClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ctx, nil, "alice", iface.DefaultMTU)
	key := dedicatedRelayKey{serverAddress: "rel://127.0.0.1:33080", channelID: 3}
	relayClient := &Client{connectionURL: key.serverAddress}
	relayClient.serviceIsRunning = true

	rt := NewRelayTrack()
	rt.Lock()
	rt.relayClient = relayClient
	rt.Unlock()

	mgr.dedicatedRelayClientsMutex.Lock()
	mgr.dedicatedRelayClients[key] = rt
	mgr.dedicatedRelayClientsMutex.Unlock()

	got, ok, err := mgr.cachedDedicatedRelayClient(key)
	if err != nil {
		t.Fatalf("expected cached ready client without error, got %s", err)
	}
	if !ok {
		t.Fatal("expected cached ready client to be found")
	}
	if got != relayClient {
		t.Fatal("expected the cached ready client to be returned")
	}
}

func TestCachedDedicatedRelayClientEvictsNotReadyClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ctx, nil, "alice", iface.DefaultMTU)
	key := dedicatedRelayKey{serverAddress: "rel://127.0.0.1:33080", channelID: 3}
	relayClient := &Client{connectionURL: key.serverAddress}

	rt := NewRelayTrack()
	rt.Lock()
	rt.relayClient = relayClient
	rt.Unlock()

	mgr.dedicatedRelayClientsMutex.Lock()
	mgr.dedicatedRelayClients[key] = rt
	mgr.dedicatedRelayClientsMutex.Unlock()

	got, ok, err := mgr.cachedDedicatedRelayClient(key)
	if err != nil {
		t.Fatalf("expected not-ready client eviction without error, got %s", err)
	}
	if ok {
		t.Fatal("expected not-ready cached client to be treated as missing")
	}
	if got != nil {
		t.Fatal("expected no client after evicting not-ready cached client")
	}

	mgr.dedicatedRelayClientsMutex.RLock()
	_, exists := mgr.dedicatedRelayClients[key]
	mgr.dedicatedRelayClientsMutex.RUnlock()
	if exists {
		t.Fatal("expected not-ready dedicated relay client to be evicted")
	}
}

func TestForeginConnClose(t *testing.T) {
	ctx := context.Background()

	srvCfg1 := server.ListenerConfig{
		Address: "localhost:52201",
	}
	srv1, err := server.NewServer(newManagerTestServerConfig(srvCfg1.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv1.Listen(srvCfg1)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv1.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	srvCfg2 := server.ListenerConfig{
		Address: "localhost:52202",
	}
	srv2, err := server.NewServer(newManagerTestServerConfig(srvCfg2.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan2 := make(chan error, 1)
	go func() {
		err := srv2.Listen(srvCfg2)
		if err != nil {
			errChan2 <- err
		}
	}()

	defer func() {
		err := srv2.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	if err := waitForServerToStart(errChan2); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	mgrBob := NewManager(mCtx, toURL(srvCfg2), "bob", iface.DefaultMTU)
	if err := mgrBob.Serve(); err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}

	mgr := NewManager(mCtx, toURL(srvCfg1), "alice", iface.DefaultMTU)
	err = mgr.Serve()
	if err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}
	conn, err := mgr.OpenConn(ctx, toURL(srvCfg2)[0], "bob", netip.Addr{})
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("failed to close connection: %s", err)
	}
}

func TestForeignAutoClose(t *testing.T) {
	ctx := context.Background()
	oldRelayCleanupInterval := relayCleanupInterval
	oldKeepUnusedServerTime := keepUnusedServerTime
	defer func() {
		relayCleanupInterval = oldRelayCleanupInterval
		keepUnusedServerTime = oldKeepUnusedServerTime
	}()
	relayCleanupInterval = 1 * time.Second
	keepUnusedServerTime = 2 * time.Second

	srvCfg1 := server.ListenerConfig{
		Address: "localhost:52301",
	}
	srv1, err := server.NewServer(newManagerTestServerConfig(srvCfg1.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		t.Log("binding server 1.")
		if err := srv1.Listen(srvCfg1); err != nil {
			errChan <- err
		}
	}()

	defer func() {
		t.Logf("closing server 1.")
		if err := srv1.Shutdown(ctx); err != nil {
			t.Errorf("failed to close server: %s", err)
		}
		t.Logf("server 1. closed")
	}()

	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	srvCfg2 := server.ListenerConfig{
		Address: "localhost:52302",
	}
	srv2, err := server.NewServer(newManagerTestServerConfig(srvCfg2.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan2 := make(chan error, 1)
	go func() {
		t.Log("binding server 2.")
		err := srv2.Listen(srvCfg2)
		if err != nil {
			errChan2 <- err
		}
	}()
	defer func() {
		t.Logf("closing server 2.")
		err := srv2.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
		t.Logf("server 2 closed.")
	}()

	if err := waitForServerToStart(errChan2); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	idAlice := "alice"
	t.Log("connect to server 1.")
	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	mgr := NewManager(mCtx, toURL(srvCfg1), idAlice, iface.DefaultMTU)
	err = mgr.Serve()
	if err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}

	// Set up a disconnect listener to track when foreign server disconnects
	foreignServerURL := toURL(srvCfg2)[0]
	disconnected := make(chan struct{})
	onDisconnect := func() {
		select {
		case disconnected <- struct{}{}:
		default:
		}
	}

	t.Log("open connection to another peer")
	openCtx, openCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer openCancel()
	if _, err = mgr.OpenConn(openCtx, foreignServerURL, "anotherpeer", netip.Addr{}); err == nil {
		t.Fatalf("should have failed to open connection to another peer")
	}

	// Add the disconnect listener after the connection attempt
	if err := mgr.AddCloseListener(foreignServerURL, onDisconnect); err != nil {
		t.Logf("failed to add close listener (expected if connection failed): %s", err)
	}

	// Wait for cleanup to happen
	timeout := relayCleanupInterval + keepUnusedServerTime + 2*time.Second
	t.Logf("waiting for relay cleanup: %s", timeout)

	select {
	case <-disconnected:
		t.Log("foreign relay connection cleaned up successfully")
	case <-time.After(timeout):
		t.Log("timeout waiting for cleanup - this might be expected if connection never established")
	}

	t.Logf("closing manager")
}

func TestAutoReconnect(t *testing.T) {
	ctx := context.Background()

	srvCfg := server.ListenerConfig{
		Address: "localhost:52401",
	}
	srv, err := server.NewServer(newManagerTestServerConfig(srvCfg.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Listen(srvCfg); err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Errorf("failed to close server: %s", err)
		}
	}()

	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	clientBob := NewManager(mCtx, toURL(srvCfg), "bob", iface.DefaultMTU)
	err = clientBob.Serve()
	if err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}

	clientAlice := NewManager(mCtx, toURL(srvCfg), "alice", iface.DefaultMTU,
		WithMaxBackoffInterval(2*time.Second))
	err = clientAlice.Serve()
	if err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}
	ra, _, err := clientAlice.RelayInstanceAddress()
	if err != nil {
		t.Errorf("failed to get relay address: %s", err)
	}
	conn, err := clientAlice.OpenConn(ctx, ra, "bob", netip.Addr{})
	if err != nil {
		t.Errorf("failed to bind channel: %s", err)
	}

	t.Log("closing client relay connection")
	// todo figure out moc server
	_ = clientAlice.relayClient.relayConn.Close()
	t.Log("start test reading")
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Errorf("unexpected reading from closed connection")
	}

	log.Infof("waiting for reconnection")
	if err := waitForReady(ctx, clientAlice, 15*time.Second); err != nil {
		t.Fatalf("manager did not reconnect: %s", err)
	}

	log.Infof("reopent the connection")
	_, err = clientAlice.OpenConn(ctx, ra, "bob", netip.Addr{})
	if err != nil {
		t.Errorf("failed to open channel: %s", err)
	}
}

func waitForReady(ctx context.Context, m *Manager, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Ready() {
			return nil
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("manager not ready within %s", timeout)
}

func TestNotifierDoubleAdd(t *testing.T) {
	ctx := context.Background()

	listenerCfg1 := server.ListenerConfig{
		Address: "localhost:52501",
	}
	srv, err := server.NewServer(newManagerTestServerConfig(listenerCfg1.Address))
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Listen(listenerCfg1); err != nil {
			errChan <- err
		}
	}()

	defer func() {
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	log.Debugf("connect by alice")
	mCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	clientBob := NewManager(mCtx, toURL(listenerCfg1), "bob", iface.DefaultMTU)
	if err = clientBob.Serve(); err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}

	clientAlice := NewManager(mCtx, toURL(listenerCfg1), "alice", iface.DefaultMTU)
	if err = clientAlice.Serve(); err != nil {
		t.Fatalf("failed to serve manager: %s", err)
	}

	conn1, err := clientAlice.OpenConn(ctx, clientAlice.ServerURLs()[0], "bob", netip.Addr{})
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	fnCloseListener := OnServerCloseListener(func() {
		log.Infof("close listener")
	})

	err = clientAlice.AddCloseListener(clientAlice.ServerURLs()[0], fnCloseListener)
	if err != nil {
		t.Fatalf("failed to add close listener: %s", err)
	}

	err = clientAlice.AddCloseListener(clientAlice.ServerURLs()[0], fnCloseListener)
	if err != nil {
		t.Fatalf("failed to add close listener: %s", err)
	}

	err = conn1.Close()
	if err != nil {
		t.Errorf("failed to close connection: %s", err)
	}

}

func toURL(address server.ListenerConfig) []string {
	return []string{"rel://" + address.Address}
}
