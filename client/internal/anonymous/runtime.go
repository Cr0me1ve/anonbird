package anonymous

import (
	"context"
	"errors"
	"reflect"
)

type Runtime struct {
	transport TransportConfig
	tor       *TorDaemon
	i2p       *I2PDaemon
}

func EnsureRuntime(ctx context.Context, transport TransportConfig) (*Runtime, error) {
	transport = NormalizeTransport(transport)
	if err := ValidateTransport(transport); err != nil {
		return nil, err
	}

	runtime := &Runtime{transport: transport}
	switch transport.Type {
	case TransportTorRelayOnly:
		daemon, err := EnsureTorDaemon(ctx, transport)
		if err != nil {
			return nil, err
		}
		runtime.tor = daemon
	case TransportI2PDatagram:
		daemon, err := EnsureI2PDaemon(ctx, transport)
		if err != nil {
			return nil, err
		}
		runtime.i2p = daemon
	default:
		return nil, ValidateTransport(transport)
	}

	if !runtime.Started() {
		return nil, nil
	}
	return runtime, nil
}

func (r *Runtime) Started() bool {
	return r != nil && ((r.tor != nil && r.tor.Started()) || (r.i2p != nil && r.i2p.Started()))
}

func (r *Runtime) Matches(transport TransportConfig) bool {
	if r == nil || !r.Started() {
		return false
	}
	return reflect.DeepEqual(runtimeTransportKey(r.transport), runtimeTransportKey(NormalizeTransport(transport)))
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	if r.tor != nil {
		closeErr = errors.Join(closeErr, r.tor.Close())
		r.tor = nil
	}
	if r.i2p != nil {
		closeErr = errors.Join(closeErr, r.i2p.Close())
		r.i2p = nil
	}
	return closeErr
}

func runtimeTransportKey(transport TransportConfig) TransportConfig {
	transport = NormalizeTransport(transport)
	switch transport.Type {
	case TransportTorRelayOnly:
		return TransportConfig{
			Type:             transport.Type,
			RequireAnonymous: true,
			TorSOCKS5:        transport.TorSOCKS5,
		}
	case TransportI2PDatagram:
		return TransportConfig{
			Type:              transport.Type,
			RequireAnonymous:  true,
			I2PSAM:            transport.I2PSAM,
			I2PTunnelLength:   transport.I2PTunnelLength,
			I2PTunnelQuantity: transport.I2PTunnelQuantity,
			I2PDaemonMode:     transport.I2PDaemonMode,
			I2PDaemonPath:     transport.I2PDaemonPath,
			I2PDataDir:        transport.I2PDataDir,
		}
	default:
		return transport
	}
}
