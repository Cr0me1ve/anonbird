package store

import (
	"sync"

	"github.com/netbirdio/netbird/shared/relay/messages"
)

type IPeer interface {
	Close()
	ID() messages.PeerID
}

type channelPeer interface {
	ChannelID() uint32
}

// Store is a thread-safe store of peers
// It is used to store the peers that are connected to the relay server
type Store struct {
	peers     map[messages.PeerID]map[uint32]IPeer
	peersLock sync.RWMutex
}

// NewStore creates a new Store instance
func NewStore() *Store {
	return &Store{
		peers: make(map[messages.PeerID]map[uint32]IPeer),
	}
}

// AddPeer adds a peer to the store
// If the same peer/channel already exists, it will be replaced and the old peer will be closed.
// Returns true if the peer/channel was replaced, false if it was added for the first time.
func (s *Store) AddPeer(peer IPeer) bool {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()
	channelID := peerChannelID(peer)
	channels, ok := s.peers[peer.ID()]
	if !ok {
		channels = make(map[uint32]IPeer)
		s.peers[peer.ID()] = channels
	}
	oldPeer, ok := channels[channelID]
	if ok {
		oldPeer.Close()
	}

	channels[channelID] = peer
	return ok
}

// DeletePeer deletes a peer from the store
func (s *Store) DeletePeer(peer IPeer) bool {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	channels, ok := s.peers[peer.ID()]
	if !ok {
		return false
	}
	channelID := peerChannelID(peer)
	dp, ok := channels[channelID]
	if !ok {
		return false
	}
	if dp != peer {
		return false
	}

	delete(channels, channelID)
	if len(channels) == 0 {
		delete(s.peers, peer.ID())
	}
	return true
}

// Peer returns a peer by its ID
func (s *Store) Peer(id messages.PeerID) (IPeer, bool) {
	s.peersLock.RLock()
	defer s.peersLock.RUnlock()

	channels, ok := s.peers[id]
	if !ok {
		return nil, false
	}
	if p, ok := channels[0]; ok {
		return p, true
	}
	for _, p := range channels {
		return p, true
	}
	return nil, false
}

// PeerChannel returns a peer by its ID and relay channel.
func (s *Store) PeerChannel(id messages.PeerID, channelID uint32) (IPeer, bool) {
	s.peersLock.RLock()
	defer s.peersLock.RUnlock()

	channels, ok := s.peers[id]
	if !ok {
		return nil, false
	}
	p, ok := channels[channelID]
	return p, ok
}

// HasPeer reports whether any relay channel for the peer is online.
func (s *Store) HasPeer(id messages.PeerID) bool {
	s.peersLock.RLock()
	defer s.peersLock.RUnlock()

	return len(s.peers[id]) > 0
}

// Peers returns all the peers in the store
func (s *Store) Peers() []IPeer {
	s.peersLock.RLock()
	defer s.peersLock.RUnlock()

	var peers []IPeer
	for _, channels := range s.peers {
		for _, p := range channels {
			peers = append(peers, p)
		}
	}
	return peers
}

func (s *Store) GetOnlinePeersAndRegisterInterest(peerIDs []messages.PeerID, listener *Listener) []messages.PeerID {
	s.peersLock.RLock()
	defer s.peersLock.RUnlock()

	onlinePeers := make([]messages.PeerID, 0, len(peerIDs))

	listener.AddInterestedPeers(peerIDs)

	// Check for currently online peers
	for _, id := range peerIDs {
		if _, ok := s.peers[id]; ok {
			onlinePeers = append(onlinePeers, id)
		}
	}

	return onlinePeers
}

func peerChannelID(peer IPeer) uint32 {
	if p, ok := peer.(channelPeer); ok {
		return p.ChannelID()
	}
	return 0
}
