package client

import (
	"context"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"

	"github.com/netbirdio/netbird/client/iface"
	"github.com/netbirdio/netbird/shared/relay/auth/allow"
	"github.com/netbirdio/netbird/shared/relay/auth/hmac"
	"github.com/netbirdio/netbird/shared/relay/client/dialer/ws"
	"github.com/netbirdio/netbird/shared/relay/messages"
	"github.com/netbirdio/netbird/util"

	"github.com/netbirdio/netbird/relay/server"
)

var (
	hmacTokenStore = &hmac.TokenStore{}
)

func TestMain(m *testing.M) {
	_ = util.InitLog("debug", util.LogConsole)
	code := m.Run()
	os.Exit(code)
}

func TestClientGetDialersWithSOCKS5UsesWebSocketOnly(t *testing.T) {
	client := NewClientWithSOCKS5("rels://relayexampleabcdefghijklmnop.onion:443", hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:9050")

	dialers := client.getDialers()
	if len(dialers) != 1 {
		t.Fatalf("expected one anonymous relay dialer, got %d", len(dialers))
	}
	if got := dialers[0].Protocol(); got != "WS" {
		t.Fatalf("expected WebSocket-only anonymous relay dialer, got %s", got)
	}
	wsDialer, ok := dialers[0].(ws.Dialer)
	if !ok {
		t.Fatalf("expected WS dialer type, got %T", dialers[0])
	}
	if wsDialer.Socks5Username != "" || wsDialer.Socks5Password != "" {
		t.Fatalf("expected primary Tor relay channel to avoid SOCKS isolation by default, got username=%q password=%q", wsDialer.Socks5Username, wsDialer.Socks5Password)
	}
	if !client.usesAnonymousRelayTransport() {
		t.Fatal("expected SOCKS5 relay client to use anonymous health-check timings")
	}
}

func TestClientGetDialersWithSOCKS5IsolatesDedicatedRelayChannelsByDefault(t *testing.T) {
	baseClient := newClientWithRelayChannel("rels://relayexampleabcdefghijklmnop.onion:443", netip.Addr{}, hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:9050", "", 0, 0, 0)
	channelClient := newClientWithRelayChannel("rels://relayexampleabcdefghijklmnop.onion:443", netip.Addr{}, hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:9050", "", 0, 0, 7)

	baseDialer := baseClient.getDialers()[0].(ws.Dialer)
	channelDialer := channelClient.getDialers()[0].(ws.Dialer)

	if baseDialer.Socks5Username != "" || baseDialer.Socks5Password != "" {
		t.Fatalf("expected primary channel to avoid SOCKS isolation by default, got username=%q password=%q", baseDialer.Socks5Username, baseDialer.Socks5Password)
	}
	if channelDialer.Socks5Username != torSOCKSAuthExtensionUsername {
		t.Fatalf("unexpected dedicated channel isolation username %q", channelDialer.Socks5Username)
	}
	if channelDialer.Socks5Password != torRelayIsolationToken(7) {
		t.Fatalf("unexpected dedicated channel isolation token %q", channelDialer.Socks5Password)
	}
}

func TestClientGetDialersWithSOCKS5IsolatesRelayChannels(t *testing.T) {
	t.Setenv(envAnonRelayTorSOCKSIsolation, "true")
	baseClient := newClientWithRelayChannel("rels://relayexampleabcdefghijklmnop.onion:443", netip.Addr{}, hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:9050", "", 0, 0, 0)
	channelClient := newClientWithRelayChannel("rels://relayexampleabcdefghijklmnop.onion:443", netip.Addr{}, hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:9050", "", 0, 0, 7)

	baseDialer := baseClient.getDialers()[0].(ws.Dialer)
	channelDialer := channelClient.getDialers()[0].(ws.Dialer)

	if baseDialer.Socks5Username != channelDialer.Socks5Username {
		t.Fatalf("expected Tor SOCKS auth extension username to stay stable, got %q and %q", baseDialer.Socks5Username, channelDialer.Socks5Username)
	}
	if baseDialer.Socks5Password == channelDialer.Socks5Password {
		t.Fatalf("expected distinct Tor isolation tokens, got %q", baseDialer.Socks5Password)
	}
	if channelDialer.Socks5Password != torRelayIsolationToken(7) {
		t.Fatalf("unexpected channel isolation token %q", channelDialer.Socks5Password)
	}
}

func TestClientGetDialersWithSOCKS5CanDisableRelayChannelIsolation(t *testing.T) {
	t.Setenv(envAnonRelayTorSOCKSIsolation, "false")
	channelClient := newClientWithRelayChannel("rels://relayexampleabcdefghijklmnop.onion:443", netip.Addr{}, hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:9050", "", 0, 0, 7)

	channelDialer := channelClient.getDialers()[0].(ws.Dialer)
	if channelDialer.Socks5Username != "" || channelDialer.Socks5Password != "" {
		t.Fatalf("expected env=false to disable dedicated channel isolation, got username=%q password=%q", channelDialer.Socks5Username, channelDialer.Socks5Password)
	}
}

func TestClientGetDialersWithI2PUsesWebSocketOnly(t *testing.T) {
	client := NewClientWithI2P("rels://relayexample.b32.i2p:443", hmacTokenStore, "alice", iface.DefaultMTU, "127.0.0.1:7656", 1, 3)

	dialers := client.getDialers()
	if len(dialers) != 1 {
		t.Fatalf("expected one anonymous relay dialer, got %d", len(dialers))
	}
	if got := dialers[0].Protocol(); got != "WS" {
		t.Fatalf("expected WebSocket-only anonymous relay dialer, got %s", got)
	}
	if !client.usesAnonymousRelayTransport() {
		t.Fatal("expected I2P relay client to use anonymous health-check timings")
	}
}

func TestOpenConnChannelAssumePeerOnlineSkipsPeerStateWait(t *testing.T) {
	client := NewClient("rel://127.0.0.1:33080", hmacTokenStore, "alice", iface.DefaultMTU)
	client.serviceIsRunning = true
	client.stateSubscription = NewPeersStateSubscription(log.NewEntry(log.New()), &mockRelayedConn{}, func([]messages.PeerID) {})

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.OpenConnChannel(cancelledCtx, "bob", 7); err == nil {
		t.Fatal("expected regular channel open to wait for peer state and fail with a cancelled context")
	}

	conn, err := client.openConnChannelAssumePeerOnline(cancelledCtx, "bob", 7)
	if err != nil {
		t.Fatalf("expected assumed-online channel open to skip peer state wait, got %s", err)
	}
	defer conn.Close()
}

func TestClientDirectRelayDoesNotUseAnonymousHealthChecks(t *testing.T) {
	client := NewClient("rel://127.0.0.1:33080", hmacTokenStore, "alice", iface.DefaultMTU)
	if client.usesAnonymousRelayTransport() {
		t.Fatal("expected direct relay client to use default health-check timings")
	}
}

// newClientTestServerConfig creates a new server config for client testing with the given address
func newClientTestServerConfig(address string) server.Config {
	return server.Config{
		Meter:          otel.Meter(""),
		ExposedAddress: "rel://" + address,
		TLSSupport:     false,
		AuthValidator:  &allow.Auth{},
	}
}

func TestClient(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50001"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		listenCfg := server.ListenerConfig{Address: serverListenAddr}
		err := srv.Listen(listenCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for server to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}
	t.Log("alice connecting to server")
	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer clientAlice.Close()

	t.Log("placeholder connecting to server")
	clientPlaceHolder := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "clientPlaceHolder", iface.DefaultMTU)
	err = clientPlaceHolder.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer clientPlaceHolder.Close()

	t.Log("Bob connecting to server")
	clientBob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "bob", iface.DefaultMTU)
	err = clientBob.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer clientBob.Close()

	t.Log("Alice open connection to Bob")
	connAliceToBob, err := clientAlice.OpenConn(ctx, "bob")
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	t.Log("Bob open connection to Alice")
	connBobToAlice, err := clientBob.OpenConn(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	payload := "hello bob, I am alice"
	_, err = connAliceToBob.Write([]byte(payload))
	if err != nil {
		t.Fatalf("failed to write to channel: %s", err)
	}
	log.Debugf("alice sent message to bob")

	buf := make([]byte, 65535)
	n, err := connBobToAlice.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from channel: %s", err)
	}
	log.Debugf("on new message from alice to bob")

	if payload != string(buf[:n]) {
		t.Fatalf("expected %s, got %s", payload, string(buf[:n]))
	}
}

func TestClientMultipleChannels(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50011"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		listenCfg := server.ListenerConfig{Address: serverListenAddr}
		if err := srv.Listen(listenCfg); err != nil {
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

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	if err := clientAlice.Connect(ctx); err != nil {
		t.Fatalf("failed to connect alice: %s", err)
	}
	defer clientAlice.Close()

	clientBob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "bob", iface.DefaultMTU)
	if err := clientBob.Connect(ctx); err != nil {
		t.Fatalf("failed to connect bob: %s", err)
	}
	defer clientBob.Close()

	aliceCh1, err := clientAlice.OpenConnChannel(ctx, "bob", 1)
	if err != nil {
		t.Fatalf("alice channel 1: %s", err)
	}
	aliceCh2, err := clientAlice.OpenConnChannel(ctx, "bob", 2)
	if err != nil {
		t.Fatalf("alice channel 2: %s", err)
	}
	bobCh1, err := clientBob.OpenConnChannel(ctx, "alice", 1)
	if err != nil {
		t.Fatalf("bob channel 1: %s", err)
	}
	bobCh2, err := clientBob.OpenConnChannel(ctx, "alice", 2)
	if err != nil {
		t.Fatalf("bob channel 2: %s", err)
	}

	if _, err := clientAlice.OpenConnChannel(ctx, "bob", 1); err != ErrConnAlreadyExists {
		t.Fatalf("expected duplicate channel error, got %v", err)
	}

	if _, err := aliceCh1.Write([]byte("flow-one")); err != nil {
		t.Fatalf("write channel 1: %s", err)
	}
	if _, err := aliceCh2.Write([]byte("flow-two")); err != nil {
		t.Fatalf("write channel 2: %s", err)
	}

	buf := make([]byte, 64)
	n, err := bobCh1.Read(buf)
	if err != nil {
		t.Fatalf("read channel 1: %s", err)
	}
	if got := string(buf[:n]); got != "flow-one" {
		t.Fatalf("expected channel 1 payload, got %q", got)
	}
	n, err = bobCh2.Read(buf)
	if err != nil {
		t.Fatalf("read channel 2: %s", err)
	}
	if got := string(buf[:n]); got != "flow-two" {
		t.Fatalf("expected channel 2 payload, got %q", got)
	}
}

func TestClientDedicatedRelayChannelDoesNotReplacePrimaryPeer(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50012"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		listenCfg := server.ListenerConfig{Address: serverListenAddr}
		if err := srv.Listen(listenCfg); err != nil {
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

	alicePrimary := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	if err := alicePrimary.Connect(ctx); err != nil {
		t.Fatalf("failed to connect alice primary: %s", err)
	}
	defer alicePrimary.Close()
	bobPrimary := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "bob", iface.DefaultMTU)
	if err := bobPrimary.Connect(ctx); err != nil {
		t.Fatalf("failed to connect bob primary: %s", err)
	}
	defer bobPrimary.Close()

	aliceStream := newClientWithRelayChannel(serverCfg.ExposedAddress, netip.Addr{}, hmacTokenStore, "alice", iface.DefaultMTU, "", "", 0, 0, 1)
	if err := aliceStream.Connect(ctx); err != nil {
		t.Fatalf("failed to connect alice channel stream: %s", err)
	}
	defer aliceStream.Close()
	bobStream := newClientWithRelayChannel(serverCfg.ExposedAddress, netip.Addr{}, hmacTokenStore, "bob", iface.DefaultMTU, "", "", 0, 0, 1)
	if err := bobStream.Connect(ctx); err != nil {
		t.Fatalf("failed to connect bob channel stream: %s", err)
	}
	defer bobStream.Close()

	alicePrimaryConn, err := alicePrimary.OpenConn(ctx, "bob")
	if err != nil {
		t.Fatalf("alice primary conn: %s", err)
	}
	bobPrimaryConn, err := bobPrimary.OpenConn(ctx, "alice")
	if err != nil {
		t.Fatalf("bob primary conn: %s", err)
	}
	aliceCh1, err := aliceStream.OpenConnChannel(ctx, "bob", 1)
	if err != nil {
		t.Fatalf("alice channel 1 conn: %s", err)
	}
	bobCh1, err := bobStream.OpenConnChannel(ctx, "alice", 1)
	if err != nil {
		t.Fatalf("bob channel 1 conn: %s", err)
	}

	if _, err := alicePrimaryConn.Write([]byte("primary")); err != nil {
		t.Fatalf("write primary: %s", err)
	}
	if _, err := aliceCh1.Write([]byte("channel-1")); err != nil {
		t.Fatalf("write channel 1: %s", err)
	}

	buf := make([]byte, 64)
	n, err := bobPrimaryConn.Read(buf)
	if err != nil {
		t.Fatalf("read primary: %s", err)
	}
	if got := string(buf[:n]); got != "primary" {
		t.Fatalf("expected primary payload, got %q", got)
	}
	n, err = bobCh1.Read(buf)
	if err != nil {
		t.Fatalf("read channel 1: %s", err)
	}
	if got := string(buf[:n]); got != "channel-1" {
		t.Fatalf("expected channel payload, got %q", got)
	}
}

func TestRegistration(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50101"
	serverCfg := newClientTestServerConfig(serverListenAddr)
	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	// wait for server to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		_ = srv.Shutdown(ctx)
		t.Fatalf("failed to connect to server: %s", err)
	}
	err = clientAlice.Close()
	if err != nil {
		t.Errorf("failed to close conn: %s", err)
	}
	err = srv.Shutdown(ctx)
	if err != nil {
		t.Errorf("failed to close server: %s", err)
	}
}

func TestRegistrationTimeout(t *testing.T) {
	ctx := context.Background()
	fakeUDPListener, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: 50201,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		t.Fatalf("failed to bind UDP server: %s", err)
	}
	defer func(fakeUDPListener *net.UDPConn) {
		_ = fakeUDPListener.Close()
	}(fakeUDPListener)

	fakeTCPListener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: 50201,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		t.Fatalf("failed to bind TCP server: %s", err)
	}
	defer func(fakeTCPListener *net.TCPListener) {
		_ = fakeTCPListener.Close()
	}(fakeTCPListener)

	clientAlice := NewClient("127.0.0.1:50201", hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err == nil {
		t.Errorf("failed to connect to server: %s", err)
	}
	log.Debugf("%s", err)
	err = clientAlice.Close()
	if err != nil {
		t.Errorf("failed to close conn: %s", err)
	}
}

func TestEcho(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50301"
	serverCfg := newClientTestServerConfig(serverListenAddr)
	idAlice := "alice"
	idBob := "bob"
	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, idAlice, iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer func() {
		err := clientAlice.Close()
		if err != nil {
			t.Errorf("failed to close Alice client: %s", err)
		}
	}()

	clientBob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, idBob, iface.DefaultMTU)
	err = clientBob.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer func() {
		err := clientBob.Close()
		if err != nil {
			t.Errorf("failed to close Bob client: %s", err)
		}
	}()

	connAliceToBob, err := clientAlice.OpenConn(ctx, idBob)
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	connBobToAlice, err := clientBob.OpenConn(ctx, idAlice)
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
}

func TestBindToUnavailabePeer(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50401"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		log.Infof("closing server")
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Errorf("failed to connect to server: %s", err)
	}
	_, err = clientAlice.OpenConn(ctx, "bob")
	if err == nil {
		t.Errorf("expected error when binding to unavailable peer, got nil")
	}

	log.Infof("closing client")
	err = clientAlice.Close()
	if err != nil {
		t.Errorf("failed to close client: %s", err)
	}
}

func TestBindReconnect(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50501"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		log.Infof("closing server")
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}

	clientBob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "bob", iface.DefaultMTU)
	err = clientBob.Connect(ctx)
	if err != nil {
		t.Errorf("failed to connect to server: %s", err)
	}

	_, err = clientAlice.OpenConn(ctx, "bob")
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	chBob, err := clientBob.OpenConn(ctx, "alice")
	if err != nil {
		t.Errorf("failed to bind channel: %s", err)
	}

	log.Infof("closing client Alice")
	err = clientAlice.Close()
	if err != nil {
		t.Errorf("failed to close client: %s", err)
	}

	clientAlice = NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Errorf("failed to connect to server: %s", err)
	}

	chAlice, err := clientAlice.OpenConn(ctx, "bob")
	if err != nil {
		t.Errorf("failed to bind channel: %s", err)
	}

	testString := "hello alice, I am bob"
	_, err = chBob.Write([]byte(testString))
	if err == nil {
		t.Errorf("expected error when writing to channel, got nil")
	}

	chBob, err = clientBob.OpenConn(ctx, "alice")
	if err != nil {
		t.Errorf("failed to bind channel: %s", err)
	}

	_, err = chBob.Write([]byte(testString))
	if err != nil {
		t.Errorf("failed to write to channel: %s", err)
	}

	buf := make([]byte, 65535)
	n, err := chAlice.Read(buf)
	if err != nil {
		t.Errorf("failed to read from channel: %s", err)
	}

	if testString != string(buf[:n]) {
		t.Errorf("expected %s, got %s", testString, string(buf[:n]))
	}

	log.Infof("closing client")
	err = clientAlice.Close()
	if err != nil {
		t.Errorf("failed to close client: %s", err)
	}
}

func TestCloseConn(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50601"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		log.Infof("closing server")
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	bob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "bob", iface.DefaultMTU)
	err = bob.Connect(ctx)
	if err != nil {
		t.Errorf("failed to connect to server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Errorf("failed to connect to server: %s", err)
	}

	conn, err := clientAlice.OpenConn(ctx, "bob")
	if err != nil {
		t.Errorf("failed to bind channel: %s", err)
	}

	log.Infof("closing connection")
	err = conn.Close()
	if err != nil {
		t.Errorf("failed to close connection: %s", err)
	}

	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Errorf("unexpected reading from closed connection")
	}

	_, err = conn.Write([]byte("hello"))
	if err == nil {
		t.Errorf("unexpected writing from closed connection")
	}
}

func TestCloseRelayConn(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50701"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	bob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "bob", iface.DefaultMTU)
	err = bob.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, "alice", iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}

	conn, err := clientAlice.OpenConn(ctx, "bob")
	if err != nil {
		t.Errorf("failed to bind channel: %s", err)
	}

	_ = clientAlice.relayConn.Close()

	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Errorf("unexpected reading from closed connection")
	}

	_, err = clientAlice.OpenConn(ctx, "bob")
	if err == nil {
		t.Errorf("unexpected opening connection to closed server")
	}
}

func TestCloseByServer(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50801"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv1, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)

	go func() {
		err := srv1.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	idAlice := "alice"
	log.Debugf("connect by alice")
	relayClient := NewClient(serverCfg.ExposedAddress, hmacTokenStore, idAlice, iface.DefaultMTU)
	if err = relayClient.Connect(ctx); err != nil {
		log.Fatalf("failed to connect to server: %s", err)
	}
	defer func() {
		if err := relayClient.Close(); err != nil {
			log.Errorf("failed to close client: %s", err)
		}
	}()

	disconnected := make(chan struct{})
	relayClient.SetOnDisconnectListener(func(_ string) {
		log.Infof("client disconnected")
		close(disconnected)
	})

	err = srv1.Shutdown(ctx)
	if err != nil {
		t.Fatalf("failed to close server: %s", err)
	}

	select {
	case <-disconnected:
	case <-time.After(3 * time.Second):
		log.Errorf("timeout waiting for client to disconnect")
	}

	_, err = relayClient.OpenConn(ctx, "bob")
	if err == nil {
		t.Errorf("unexpected opening connection to closed server")
	}
}

func TestCloseByClient(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:50901"
	serverCfg := newClientTestServerConfig(serverListenAddr)

	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	idAlice := "alice"
	log.Debugf("connect by alice")
	relayClient := NewClient(serverCfg.ExposedAddress, hmacTokenStore, idAlice, iface.DefaultMTU)
	err = relayClient.Connect(ctx)
	if err != nil {
		log.Fatalf("failed to connect to server: %s", err)
	}

	err = relayClient.Close()
	if err != nil {
		t.Errorf("failed to close client: %s", err)
	}

	_, err = relayClient.OpenConn(ctx, "bob")
	if err == nil {
		t.Errorf("unexpected opening connection to closed server")
	}

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("failed to close server: %s", err)
	}
}

func TestCloseNotDrainedChannel(t *testing.T) {
	ctx := context.Background()
	serverListenAddr := "127.0.0.1:51001"
	serverCfg := newClientTestServerConfig(serverListenAddr)
	idAlice := "alice"
	idBob := "bob"
	srvCfg := server.ListenerConfig{Address: serverListenAddr}
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("failed to create server: %s", err)
	}
	errChan := make(chan error, 1)
	go func() {
		err := srv.Listen(srvCfg)
		if err != nil {
			errChan <- err
		}
	}()

	defer func() {
		err := srv.Shutdown(ctx)
		if err != nil {
			t.Errorf("failed to close server: %s", err)
		}
	}()

	// wait for servers to start
	if err := waitForServerToStart(errChan); err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	clientAlice := NewClient(serverCfg.ExposedAddress, hmacTokenStore, idAlice, iface.DefaultMTU)
	err = clientAlice.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer func() {
		err := clientAlice.Close()
		if err != nil {
			t.Errorf("failed to close Alice client: %s", err)
		}
	}()

	clientBob := NewClient(serverCfg.ExposedAddress, hmacTokenStore, idBob, iface.DefaultMTU)
	err = clientBob.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect to server: %s", err)
	}
	defer func() {
		err := clientBob.Close()
		if err != nil {
			t.Errorf("failed to close Bob client: %s", err)
		}
	}()

	connAliceToBob, err := clientAlice.OpenConn(ctx, idBob)
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	connBobToAlice, err := clientBob.OpenConn(ctx, idAlice)
	if err != nil {
		t.Fatalf("failed to bind channel: %s", err)
	}

	payload := "hello bob, I am alice"
	// the internal channel buffer size is 2. So we should overflow it
	for i := 0; i < 5; i++ {
		_, err = connAliceToBob.Write([]byte(payload))
		if err != nil {
			t.Fatalf("failed to write to channel: %s", err)
		}

	}

	// wait for delivery
	time.Sleep(1 * time.Second)
	err = connBobToAlice.Close()
	if err != nil {
		t.Errorf("failed to close channel: %s", err)
	}
}

func waitForServerToStart(errChan chan error) error {
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	case <-time.After(300 * time.Millisecond):
		return nil
	}
	return nil
}
