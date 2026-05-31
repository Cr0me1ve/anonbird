package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
	"github.com/netbirdio/netbird/client/proto"
)

func TestDefaultAnonymousModeAppliedToConfigInput(t *testing.T) {
	restore := saveAnonymousPolicyGlobals()
	t.Cleanup(restore)

	anonymousMode = true
	noAnonymousMode = false
	anonymousTransport = anonymous.TransportTorRelayOnly
	torSOCKS5 = anonymous.DefaultTorSOCKS5

	ic := &profilemanager.ConfigInput{}
	applyAnonymousConfigInput(ic)

	require.NotNil(t, ic.AnonymousMode)
	require.True(t, *ic.AnonymousMode)
	require.NotNil(t, ic.AnonymousTransport)
	require.Equal(t, anonymous.TransportTorRelayOnly, ic.AnonymousTransport.Type)
	require.Equal(t, anonymous.DefaultTorSOCKS5, ic.AnonymousTransport.TorSOCKS5)
}

func TestDefaultAnonymousModeAppliedToLoginRequest(t *testing.T) {
	restore := saveAnonymousPolicyGlobals()
	t.Cleanup(restore)

	anonymousMode = true
	noAnonymousMode = false
	anonymousTransport = anonymous.TransportTorRelayOnly
	torSOCKS5 = anonymous.DefaultTorSOCKS5

	req := &proto.LoginRequest{}
	applyAnonymousLoginRequest(req)

	require.NotNil(t, req.AnonymousMode)
	require.True(t, *req.AnonymousMode)
	require.NotNil(t, req.AnonymousTransport)
	require.Equal(t, anonymous.TransportTorRelayOnly, *req.AnonymousTransport)
	require.NotNil(t, req.TorSocks5)
	require.Equal(t, anonymous.DefaultTorSOCKS5, *req.TorSocks5)
}

func TestUnsafeClearnetRequiresExplicitConfirmation(t *testing.T) {
	restore := saveAnonymousPolicyGlobals()
	t.Cleanup(restore)

	anonymousMode = true
	noAnonymousMode = true
	allowUnsafeClearnet = false
	unsafeClearnetAck = false

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(""))

	err := validateAnonymousModePolicy(cmd)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--allow-unsafe-clearnet")
	require.Contains(t, stderr.String(), "WARNING: You are trying to connect without AnonBird anonymous mode")
	require.Contains(t, stderr.String(), "real IP address")
}

func TestUnsafeClearnetAllowsExplicitNonInteractiveConfirmation(t *testing.T) {
	restore := saveAnonymousPolicyGlobals()
	t.Cleanup(restore)

	anonymousMode = true
	noAnonymousMode = true
	allowUnsafeClearnet = true
	unsafeClearnetAck = true

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(""))

	require.NoError(t, validateAnonymousModePolicy(cmd))
	require.Contains(t, stderr.String(), "WARNING: You are trying to connect without AnonBird anonymous mode")
}

func saveAnonymousPolicyGlobals() func() {
	origAnonymousMode := anonymousMode
	origNoAnonymousMode := noAnonymousMode
	origAllowUnsafeClearnet := allowUnsafeClearnet
	origUnsafeClearnetAck := unsafeClearnetAck
	origAnonymousTransport := anonymousTransport
	origTorSOCKS5 := torSOCKS5

	return func() {
		anonymousMode = origAnonymousMode
		noAnonymousMode = origNoAnonymousMode
		allowUnsafeClearnet = origAllowUnsafeClearnet
		unsafeClearnetAck = origUnsafeClearnetAck
		anonymousTransport = origAnonymousTransport
		torSOCKS5 = origTorSOCKS5
	}
}
