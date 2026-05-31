package messages

import (
	"testing"
)

func TestMarshalHelloMsg(t *testing.T) {
	peerID := HashID("abdFAaBcawquEiCMzAabYosuUaGLtSNhKxz+")
	msg, err := MarshalHelloMsg(peerID, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineClientMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if msgType != MsgTypeHello {
		t.Errorf("expected %d, got %d", MsgTypeHello, msgType)
	}

	receivedPeerID, _, err := UnmarshalHelloMsg(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if receivedPeerID.String() != peerID.String() {
		t.Errorf("expected %s, got %s", peerID, receivedPeerID)
	}
}

func TestMarshalAuthMsg(t *testing.T) {
	peerID := HashID("abdFAaBcawquEiCMzAabYosuUaGLtSNhKxz+")
	msg, err := MarshalAuthMsg(peerID, []byte{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineClientMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if msgType != MsgTypeAuth {
		t.Errorf("expected %d, got %d", MsgTypeAuth, msgType)
	}

	receivedPeerID, _, err := UnmarshalAuthMsg(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if receivedPeerID.String() != peerID.String() {
		t.Errorf("expected %s, got %s", peerID, receivedPeerID)
	}
}

func TestMarshalAuthChannelMsg(t *testing.T) {
	peerID := HashID("abdFAaBcawquEiCMzAabYosuUaGLtSNhKxz+")
	authPayload := []byte("auth-payload")
	const channelID uint32 = 3

	msg, err := MarshalAuthChannelMsg(peerID, channelID, authPayload)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineClientMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if msgType != MsgTypeAuthChannel {
		t.Errorf("expected %d, got %d", MsgTypeAuthChannel, msgType)
	}

	receivedPeerID, receivedChannelID, receivedPayload, err := UnmarshalAuthChannelMsg(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if receivedPeerID.String() != peerID.String() {
		t.Errorf("expected %s, got %s", peerID, receivedPeerID)
	}
	if receivedChannelID != channelID {
		t.Errorf("expected channel %d, got %d", channelID, receivedChannelID)
	}
	if string(receivedPayload) != string(authPayload) {
		t.Errorf("expected %s, got %s", authPayload, receivedPayload)
	}
}

func TestUnmarshalAuthChannelMsgAcceptsLegacyAuth(t *testing.T) {
	peerID := HashID("abdFAaBcawquEiCMzAabYosuUaGLtSNhKxz+")
	authPayload := []byte("auth-payload")

	msg, err := MarshalAuthMsg(peerID, authPayload)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	receivedPeerID, channelID, receivedPayload, err := UnmarshalAuthChannelMsg(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if receivedPeerID.String() != peerID.String() {
		t.Errorf("expected %s, got %s", peerID, receivedPeerID)
	}
	if channelID != 0 {
		t.Errorf("expected legacy channel 0, got %d", channelID)
	}
	if string(receivedPayload) != string(authPayload) {
		t.Errorf("expected %s, got %s", authPayload, receivedPayload)
	}
}

func TestMarshalAuthResponse(t *testing.T) {
	address := "myaddress"
	msg, err := MarshalAuthResponse(address)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineServerMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if msgType != MsgTypeAuthResponse {
		t.Errorf("expected %d, got %d", MsgTypeAuthResponse, msgType)
	}

	respAddr, err := UnmarshalAuthResponse(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if respAddr != address {
		t.Errorf("expected %s, got %s", address, respAddr)
	}
}

func TestMarshalTransportMsg(t *testing.T) {
	peerID := HashID("abdFAaBcawquEiCMzAabYosuUaGLtSNhKxz+")
	payload := []byte("payload")
	msg, err := MarshalTransportMsg(peerID, payload)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineClientMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if msgType != MsgTypeTransport {
		t.Errorf("expected %d, got %d", MsgTypeTransport, msgType)
	}

	uPeerID, err := UnmarshalTransportID(msg)
	if err != nil {
		t.Fatalf("failed to unmarshal transport id: %v", err)
	}

	if uPeerID.String() != peerID.String() {
		t.Errorf("expected %s, got %s", peerID, uPeerID)
	}

	id, respPayload, err := UnmarshalTransportMsg(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if id.String() != peerID.String() {
		t.Errorf("expected: '%s', got: '%s'", peerID, id)
	}

	if string(respPayload) != string(payload) {
		t.Errorf("expected %s, got %s", payload, respPayload)
	}

	channelID, chPayload, err := func() (uint32, []byte, error) {
		id, channelID, payload, err := UnmarshalTransportChannelMsg(msg)
		if err != nil {
			return 0, nil, err
		}
		if id.String() != peerID.String() {
			t.Errorf("expected: '%s', got: '%s'", peerID, id)
		}
		return channelID, payload, nil
	}()
	if err != nil {
		t.Fatalf("channel-compatible unmarshal: %v", err)
	}
	if channelID != 0 {
		t.Errorf("expected legacy channel 0, got %d", channelID)
	}
	if string(chPayload) != string(payload) {
		t.Errorf("expected %s, got %s", payload, chPayload)
	}
}

func TestMarshalTransportChannelMsg(t *testing.T) {
	peerID := HashID("abdFAaBcawquEiCMzAabYosuUaGLtSNhKxz+")
	payload := []byte("payload")
	const channelID uint32 = 7

	msg, err := MarshalTransportChannelMsg(peerID, channelID, payload)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineClientMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if msgType != MsgTypeTransportChannel {
		t.Errorf("expected %d, got %d", MsgTypeTransportChannel, msgType)
	}

	uPeerID, err := UnmarshalTransportID(msg)
	if err != nil {
		t.Fatalf("failed to unmarshal transport id: %v", err)
	}
	if uPeerID.String() != peerID.String() {
		t.Errorf("expected %s, got %s", peerID, uPeerID)
	}

	id, gotChannelID, respPayload, err := UnmarshalTransportChannelMsg(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if id.String() != peerID.String() {
		t.Errorf("expected: '%s', got: '%s'", peerID, id)
	}
	if gotChannelID != channelID {
		t.Errorf("expected channel %d, got %d", channelID, gotChannelID)
	}
	if string(respPayload) != string(payload) {
		t.Errorf("expected %s, got %s", payload, respPayload)
	}
}

func TestMarshalHealthcheck(t *testing.T) {
	msg := MarshalHealthcheck()

	_, err := ValidateVersion(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	msgType, err := DetermineServerMessageType(msg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if msgType != MsgTypeHealthCheck {
		t.Errorf("expected %d, got %d", MsgTypeHealthCheck, msgType)
	}
}
