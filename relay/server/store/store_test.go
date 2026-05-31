package store

import (
	"testing"

	"github.com/netbirdio/netbird/shared/relay/messages"
)

type MocPeer struct {
	id        messages.PeerID
	channelID uint32
}

func (m *MocPeer) Close() {

}

func (m *MocPeer) ID() messages.PeerID {
	return m.id
}

func (m *MocPeer) ChannelID() uint32 {
	return m.channelID
}

func TestStore_DeletePeer(t *testing.T) {
	s := NewStore()

	pID := messages.HashID("peer_one")
	p := &MocPeer{id: pID}
	s.AddPeer(p)
	s.DeletePeer(p)
	if _, ok := s.Peer(pID); ok {
		t.Errorf("peer was not deleted")
	}
}

func TestStore_AllowsMultipleChannelsForSamePeer(t *testing.T) {
	s := NewStore()

	pID := messages.HashID("peer_one")
	p0 := &MocPeer{id: pID, channelID: 0}
	p1 := &MocPeer{id: pID, channelID: 1}

	s.AddPeer(p0)
	s.AddPeer(p1)

	if got := len(s.Peers()); got != 2 {
		t.Fatalf("expected two peer channels, got %d", got)
	}
	if got, ok := s.PeerChannel(pID, 0); !ok || got != p0 {
		t.Fatalf("expected channel 0 peer")
	}
	if got, ok := s.PeerChannel(pID, 1); !ok || got != p1 {
		t.Fatalf("expected channel 1 peer")
	}

	s.DeletePeer(p1)
	if _, ok := s.Peer(pID); !ok {
		t.Fatalf("channel 0 should remain online after deleting channel 1")
	}
	if _, ok := s.PeerChannel(pID, 1); ok {
		t.Fatalf("channel 1 should be deleted")
	}
}

func TestStore_DeleteDeprecatedPeer(t *testing.T) {
	s := NewStore()

	pID1 := messages.HashID("peer_one")
	pID2 := messages.HashID("peer_one")

	p1 := &MocPeer{id: pID1}
	p2 := &MocPeer{id: pID2}

	s.AddPeer(p1)
	s.AddPeer(p2)
	s.DeletePeer(p1)

	if _, ok := s.Peer(pID2); !ok {
		t.Errorf("second peer was deleted")
	}
}
