package peer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/client/iface/device"
	"github.com/netbirdio/netbird/client/internal/anonymous"
	relayClient "github.com/netbirdio/netbird/shared/relay/client"
)

const relayMultipathDefaultChannels = 4

type RelayConnInfo struct {
	relayedConn     net.Conn
	rosenpassPubKey []byte
	rosenpassAddr   string
}

type WorkerRelay struct {
	peerCtx      context.Context
	log          *log.Entry
	isController bool
	config       ConnConfig
	conn         *Conn
	relayManager *relayClient.Manager

	relayedConn net.Conn
	relayLock   sync.Mutex

	relaySupportedOnRemotePeer atomic.Bool
}

func NewWorkerRelay(ctx context.Context, log *log.Entry, ctrl bool, config ConnConfig, conn *Conn, relayManager *relayClient.Manager) *WorkerRelay {
	r := &WorkerRelay{
		peerCtx:      ctx,
		log:          log,
		isController: ctrl,
		config:       config,
		conn:         conn,
		relayManager: relayManager,
	}
	return r
}

func (w *WorkerRelay) OnNewOffer(remoteOfferAnswer *OfferAnswer) {
	if !w.isRelaySupported(remoteOfferAnswer) {
		w.log.Infof("Relay is not supported by remote peer")
		w.relaySupportedOnRemotePeer.Store(false)
		return
	}
	w.relaySupportedOnRemotePeer.Store(true)

	// the relayManager will return with error in case if the connection has lost with relay server
	currentRelayAddress, _, err := w.relayManager.RelayInstanceAddress()
	if err != nil {
		w.log.Errorf("failed to handle new offer: %s", err)
		return
	}

	srv := w.preferredRelayServer(currentRelayAddress, remoteOfferAnswer.RelaySrvAddress)
	var serverIP netip.Addr
	if srv == remoteOfferAnswer.RelaySrvAddress {
		serverIP = remoteOfferAnswer.RelaySrvIP
	}

	relayedConn, err := w.relayManager.OpenConn(w.peerCtx, srv, w.config.Key, serverIP)
	if err != nil {
		if errors.Is(err, relayClient.ErrConnAlreadyExists) {
			w.log.Debugf("handled offer by reusing existing relay connection")
			return
		}
		w.log.Errorf("failed to open connection via Relay: %s", err)
		return
	}
	relayedConn, err = w.wrapRelayMultipathConn(srv, serverIP, relayedConn)
	if err != nil {
		w.log.Errorf("failed to enable relay multipath: %s", err)
		return
	}

	w.relayLock.Lock()
	w.relayedConn = relayedConn
	w.relayLock.Unlock()

	err = w.relayManager.AddCloseListener(srv, w.onRelayClientDisconnected)
	if err != nil {
		log.Errorf("failed to add close listener: %s", err)
		_ = relayedConn.Close()
		return
	}

	w.log.Debugf("peer conn opened via Relay: %s", srv)
	go w.conn.onRelayConnectionIsReady(RelayConnInfo{
		relayedConn:     relayedConn,
		rosenpassPubKey: remoteOfferAnswer.RosenpassPubKey,
		rosenpassAddr:   remoteOfferAnswer.RosenpassAddr,
	})
}

func (w *WorkerRelay) RelayInstanceAddress() (string, netip.Addr, error) {
	return w.relayManager.RelayInstanceAddress()
}

func (w *WorkerRelay) IsRelayConnectionSupportedWithPeer() bool {
	return w.relaySupportedOnRemotePeer.Load() && w.RelayIsSupportedLocally()
}

func (w *WorkerRelay) RelayIsSupportedLocally() bool {
	return w.relayManager.HasRelayAddress()
}

func (w *WorkerRelay) CloseConn() {
	w.relayLock.Lock()
	defer w.relayLock.Unlock()
	if w.relayedConn == nil {
		return
	}

	if err := w.relayedConn.Close(); err != nil {
		w.log.Warnf("failed to close relay connection: %v", err)
	}
}

func (w *WorkerRelay) isRelaySupported(answer *OfferAnswer) bool {
	if !w.relayManager.HasRelayAddress() {
		return false
	}
	return answer.RelaySrvAddress != ""
}

func (w *WorkerRelay) preferredRelayServer(myRelayAddress, remoteRelayAddress string) string {
	if w.isController {
		return myRelayAddress
	}
	return remoteRelayAddress
}

func (w *WorkerRelay) onRelayClientDisconnected() {
	go w.conn.onRelayDisconnected()
}

func (w *WorkerRelay) wrapRelayMultipathConn(serverAddress string, serverIP netip.Addr, primaryConn net.Conn) (net.Conn, error) {
	channelCount := w.relayMultipathChannelCount()
	if channelCount <= 1 {
		return primaryConn, nil
	}

	provider, ok := w.config.WgConfig.WgInterface.(interface {
		GetDevice() *device.FilteredDevice
	})
	if !ok {
		_ = primaryConn.Close()
		return nil, fmt.Errorf("userspace filtered device is not available")
	}
	filteredDevice := provider.GetDevice()
	if filteredDevice == nil {
		_ = primaryConn.Close()
		return nil, fmt.Errorf("userspace filtered device is not initialized")
	}

	channels := []relayMultipathChannel{{id: 0, conn: primaryConn}}
	for channelID := 1; channelID < channelCount; channelID++ {
		channelConn, err := w.relayManager.OpenConnChannel(w.peerCtx, serverAddress, w.config.Key, serverIP, uint32(channelID))
		if err != nil {
			w.log.Warnf("anonymous relay multipath channel %d unavailable: %s", channelID, err)
			continue
		}
		channels = append(channels, relayMultipathChannel{id: uint32(channelID), conn: channelConn})
	}
	if len(channels) == 1 {
		w.log.Warnf("anonymous relay multipath unavailable, continuing with primary relay channel")
		return primaryConn, nil
	}

	multipathConn := newRelayMultipathConn(channels, w.config.WgConfig.AllowedIps, nil)
	multipathConn.setUnregisterObserver(filteredDevice.AddPacketObserver(multipathConn))
	w.log.Infof("anonymous relay multipath enabled with %d/%d channels", len(channels), channelCount)
	return multipathConn, nil
}

func (w *WorkerRelay) relayMultipathChannelCount() int {
	if !w.config.AnonymousMode {
		return 1
	}
	transport := anonymous.NormalizeTransport(w.config.AnonymousTransport)
	if transport.Type != anonymous.TransportTorRelayOnly {
		return 1
	}
	return relayMultipathDefaultChannels
}
