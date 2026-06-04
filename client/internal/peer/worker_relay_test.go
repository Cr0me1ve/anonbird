package peer

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/anonymous"
)

func TestRelayMultipathChannelCountUsesEnvOverride(t *testing.T) {
	t.Setenv(envAnonRelayMultipathChannels, "8")
	worker := newTestWorkerRelayForMultipath()

	require.Equal(t, 8, worker.relayMultipathChannelCount())
}

func TestRelayMultipathChannelCountCapsEnvOverride(t *testing.T) {
	t.Setenv(envAnonRelayMultipathChannels, "64")
	worker := newTestWorkerRelayForMultipath()

	require.Equal(t, relayMultipathMaxChannels, worker.relayMultipathChannelCount())
}

func TestRelayMultipathChannelCountRejectsInvalidEnvOverride(t *testing.T) {
	t.Setenv(envAnonRelayMultipathChannels, "nope")
	worker := newTestWorkerRelayForMultipath()

	require.Equal(t, relayMultipathDefaultChannels, worker.relayMultipathChannelCount())
}

func newTestWorkerRelayForMultipath() *WorkerRelay {
	logger := log.New()
	logger.SetLevel(log.FatalLevel)
	return &WorkerRelay{
		log: log.NewEntry(logger),
		config: ConnConfig{
			AnonymousMode:      true,
			AnonymousTransport: anonymous.TransportConfig{Type: anonymous.TransportTorRelayOnly},
		},
	}
}
