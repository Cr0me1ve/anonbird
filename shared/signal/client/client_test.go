package client

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/netbirdio/netbird/shared/signal/proto"
)

func TestMarshalCredentialAnonymousModeOmitsRelayServerIP(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	require.NoError(t, err)

	msg, err := MarshalCredential(key, "remote", CredentialPayload{
		Type:          proto.Body_OFFER,
		Credential:    &Credential{UFrag: "u", Pwd: "p"},
		RelaySrvIP:    netip.MustParseAddr("93.177.116.58"),
		AnonymousMode: true,
	})
	require.NoError(t, err)

	require.Empty(t, msg.GetBody().GetRelayServerIP())
}

func TestMarshalCredentialNormalModeIncludesRelayServerIP(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	require.NoError(t, err)

	msg, err := MarshalCredential(key, "remote", CredentialPayload{
		Type:       proto.Body_OFFER,
		Credential: &Credential{UFrag: "u", Pwd: "p"},
		RelaySrvIP: netip.MustParseAddr("93.177.116.58"),
	})
	require.NoError(t, err)

	require.NotEmpty(t, msg.GetBody().GetRelayServerIP())
}
