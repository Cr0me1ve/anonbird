package profilemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/netbirdio/netbird/client/iface"
	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/client/internal/routemanager/dynamic"
	"github.com/netbirdio/netbird/util"
)

type mockMgmProber struct{}

func (m *mockMgmProber) HealthCheck() error {
	return nil
}

func (m *mockMgmProber) Close() error { return nil }

func TestGetConfig(t *testing.T) {
	// case 1: new default config has to be generated
	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
	})
	if err != nil {
		return
	}

	assert.Equal(t, config.ManagementURL.String(), DefaultManagementURL)
	assert.Equal(t, config.AdminURL.String(), DefaultAdminURL)

	managementURL := "https://test.management.url:33071"
	adminURL := "https://app.admin.url:443"
	path := filepath.Join(t.TempDir(), "config.json")
	preSharedKey := "preSharedKey"

	// case 2: new config has to be generated
	config, err = UpdateOrCreateConfig(ConfigInput{
		ManagementURL: managementURL,
		AdminURL:      adminURL,
		ConfigPath:    path,
		PreSharedKey:  &preSharedKey,
	})
	if err != nil {
		return
	}

	assert.Equal(t, config.ManagementURL.String(), managementURL)
	assert.Equal(t, config.PreSharedKey, preSharedKey)

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Errorf("config file was expected to be created under path %s", path)
	}

	// case 3: existing config -> fetch it
	config, err = UpdateOrCreateConfig(ConfigInput{
		ManagementURL: managementURL,
		AdminURL:      adminURL,
		ConfigPath:    path,
		PreSharedKey:  &preSharedKey,
	})
	if err != nil {
		return
	}

	assert.Equal(t, config.ManagementURL.String(), managementURL)
	assert.Equal(t, config.PreSharedKey, preSharedKey)

	// case 4: existing config, but new managementURL has been provided -> update config
	newManagementURL := "https://test.newManagement.url:33071"
	config, err = UpdateOrCreateConfig(ConfigInput{
		ManagementURL: newManagementURL,
		AdminURL:      adminURL,
		ConfigPath:    path,
		PreSharedKey:  &preSharedKey,
	})
	if err != nil {
		return
	}

	assert.Equal(t, config.ManagementURL.String(), newManagementURL)
	assert.Equal(t, config.PreSharedKey, preSharedKey)

	// read once more to make sure that config file has been updated with the new management URL
	readConf, err := util.ReadJson(path, config)
	if err != nil {
		return
	}
	assert.Equal(t, readConf.(*Config).ManagementURL.String(), newManagementURL)
}

func TestExtraIFaceBlackList(t *testing.T) {
	extraIFaceBlackList := []string{"eth1"}
	path := filepath.Join(t.TempDir(), "config.json")
	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:          path,
		ExtraIFaceBlackList: extraIFaceBlackList,
	})
	if err != nil {
		return
	}

	assert.Contains(t, config.IFaceBlackList, "eth1")
	readConf, err := util.ReadJson(path, config)
	if err != nil {
		return
	}

	assert.Contains(t, readConf.(*Config).IFaceBlackList, "eth1")
}

func TestHiddenPreSharedKey(t *testing.T) {
	hidden := "**********"
	samplePreSharedKey := "mysecretpresharedkey"
	tests := []struct {
		name         string
		preSharedKey *string
		want         string
	}{
		{"nil", nil, ""},
		{"hidden", &hidden, ""},
		{"filled", &samplePreSharedKey, samplePreSharedKey},
	}

	// generate default cfg
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	_, _ = UpdateOrCreateConfig(ConfigInput{
		ConfigPath: cfgFile,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := UpdateOrCreateConfig(ConfigInput{
				ConfigPath:   cfgFile,
				PreSharedKey: tt.preSharedKey,
			})
			if err != nil {
				t.Fatalf("failed to get cfg: %s", err)
			}

			if cfg.PreSharedKey != tt.want {
				t.Fatalf("invalid preshared key: '%s', expected: '%s' ", cfg.PreSharedKey, tt.want)
			}
		})
	}
}

func TestNewProfileDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath: configPath,
	})
	require.NoError(t, err, "should create new config")

	assert.Equal(t, DefaultManagementURL, config.ManagementURL.String(), "ManagementURL should have default")
	assert.Equal(t, DefaultAdminURL, config.AdminURL.String(), "AdminURL should have default")
	assert.NotEmpty(t, config.PrivateKey, "PrivateKey should be generated")
	assert.NotEmpty(t, config.SSHKey, "SSHKey should be generated")
	assert.Equal(t, iface.WgInterfaceDefault, config.WgIface, "WgIface should have default")
	assert.Equal(t, iface.DefaultWgPort, config.WgPort, "WgPort should default to 51820")
	assert.Equal(t, uint16(iface.DefaultMTU), config.MTU, "MTU should have default")
	assert.Equal(t, dynamic.DefaultInterval, config.DNSRouteInterval, "DNSRouteInterval should have default")
	assert.NotNil(t, config.ServerSSHAllowed, "ServerSSHAllowed should be set")
	assert.NotNil(t, config.DisableNotifications, "DisableNotifications should be set")
	assert.NotEmpty(t, config.IFaceBlackList, "IFaceBlackList should have defaults")

	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		assert.NotNil(t, config.NetworkMonitor, "NetworkMonitor should be set on Windows/macOS")
		assert.True(t, *config.NetworkMonitor, "NetworkMonitor should be enabled by default on Windows/macOS")
	}
}

func TestWireguardPortZeroExplicit(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	// Create a new profile with explicit port 0 (random port)
	explicitZero := 0
	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:    configPath,
		WireguardPort: &explicitZero,
	})
	require.NoError(t, err, "should create config with explicit port 0")

	assert.Equal(t, 0, config.WgPort, "WgPort should be 0 when explicitly set by user")

	// Verify it persists
	readConfig, err := GetConfig(configPath)
	require.NoError(t, err)
	assert.Equal(t, 0, readConfig.WgPort, "WgPort should remain 0 after reading from file")
}

func TestAnonymousModeConfig(t *testing.T) {
	enabled := true
	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:    filepath.Join(t.TempDir(), "config.json"),
		ManagementURL: "http://managementexampleabcdefghijklmnop.onion",
		AnonymousMode: &enabled,
	})
	require.NoError(t, err)

	require.True(t, config.AnonymousMode)
	require.Equal(t, anonymous.TransportTorRelayOnly, config.AnonymousTransport.Type)
	require.Equal(t, anonymous.DefaultTorSOCKS5, config.AnonymousTransport.TorSOCKS5)
	require.True(t, config.AnonymousTransport.RequireAnonymous)
	require.True(t, config.DisableClientRoutes)
	require.True(t, config.DisableServerRoutes)
	require.True(t, config.BlockLANAccess)
	require.False(t, config.BlockInbound)
	require.False(t, config.LazyConnectionEnabled)
}

func TestAnonymousModeI2PConfig(t *testing.T) {
	enabled := true
	transport := anonymous.TransportConfig{Type: anonymous.TransportI2PDatagram}
	configPath := filepath.Join(t.TempDir(), "config.json")
	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:         configPath,
		ManagementURL:      "http://managementexample.b32.i2p",
		AnonymousMode:      &enabled,
		AnonymousTransport: &transport,
	})
	require.NoError(t, err)

	require.True(t, config.AnonymousMode)
	require.Equal(t, anonymous.TransportI2PDatagram, config.AnonymousTransport.Type)
	require.Equal(t, anonymous.DefaultI2PSAM, config.AnonymousTransport.I2PSAM)
	require.Equal(t, uint8(anonymous.DefaultI2PTunnelLength), config.AnonymousTransport.I2PTunnelLength)
	require.Equal(t, uint8(anonymous.DefaultI2PTunnelQuantity), config.AnonymousTransport.I2PTunnelQuantity)
	require.Equal(t, anonymous.DefaultI2PDaemonMode, config.AnonymousTransport.I2PDaemonMode)
	require.Equal(t, anonymous.DefaultI2PDaemonPath, config.AnonymousTransport.I2PDaemonPath)
	require.Empty(t, config.AnonymousTransport.I2PDataDir)
	require.True(t, config.AnonymousTransport.RequireAnonymous)
}

func TestAnonymousModeI2PCustomTunnelConfig(t *testing.T) {
	enabled := true
	transport := anonymous.TransportConfig{
		Type:                  anonymous.TransportI2PDatagram,
		I2PSAM:                "127.0.0.1:17656",
		I2PTunnelLength:       2,
		I2PTunnelQuantity:     4,
		I2PDestinationPublic:  "public-destination",
		I2PDestinationPrivate: "private-destination",
		I2PDaemonMode:         anonymous.I2PDaemonManaged,
		I2PDaemonPath:         "/usr/local/bin/i2pd",
		I2PDataDir:            "/tmp/anonbird-i2pd",
	}
	config, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:         filepath.Join(t.TempDir(), "config.json"),
		ManagementURL:      "http://managementexample.b32.i2p",
		AnonymousMode:      &enabled,
		AnonymousTransport: &transport,
	})
	require.NoError(t, err)

	require.Equal(t, anonymous.TransportI2PDatagram, config.AnonymousTransport.Type)
	require.Equal(t, "127.0.0.1:17656", config.AnonymousTransport.I2PSAM)
	require.Equal(t, uint8(2), config.AnonymousTransport.I2PTunnelLength)
	require.Equal(t, uint8(4), config.AnonymousTransport.I2PTunnelQuantity)
	require.Equal(t, "public-destination", config.AnonymousTransport.I2PDestinationPublic)
	require.Equal(t, "private-destination", config.AnonymousTransport.I2PDestinationPrivate)
	require.Equal(t, anonymous.I2PDaemonManaged, config.AnonymousTransport.I2PDaemonMode)
	require.Equal(t, "/usr/local/bin/i2pd", config.AnonymousTransport.I2PDaemonPath)
	require.Equal(t, "/tmp/anonbird-i2pd", config.AnonymousTransport.I2PDataDir)
}

func TestAnonymousModeRejectsClearnetManagement(t *testing.T) {
	enabled := true
	_, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:    filepath.Join(t.TempDir(), "config.json"),
		ManagementURL: "https://api.netbird.io",
		AnonymousMode: &enabled,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "anonymous mode violation")
}

func TestAnonymousModeRejectsTransportMismatch(t *testing.T) {
	enabled := true
	transport := anonymous.TransportConfig{Type: anonymous.TransportI2PDatagram}
	_, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:         filepath.Join(t.TempDir(), "config.json"),
		ManagementURL:      "http://managementexampleabcdefghijklmnop.onion",
		AnonymousMode:      &enabled,
		AnonymousTransport: &transport,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "i2p-datagram")
}

func TestAnonymousModeRejectsNATExternalIPs(t *testing.T) {
	enabled := true
	_, err := UpdateOrCreateConfig(ConfigInput{
		ConfigPath:     filepath.Join(t.TempDir(), "config.json"),
		ManagementURL:  "http://managementexampleabcdefghijklmnop.onion",
		AnonymousMode:  &enabled,
		NATExternalIPs: []string{"93.177.116.58"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "NAT external IP mappings")
}

func TestWireguardPortDefaultVsExplicit(t *testing.T) {
	tests := []struct {
		name          string
		wireguardPort *int
		expectedPort  int
		description   string
	}{
		{
			name:          "no port specified uses default",
			wireguardPort: nil,
			expectedPort:  iface.DefaultWgPort,
			description:   "When user doesn't specify port, default to 51820",
		},
		{
			name:          "explicit zero for random port",
			wireguardPort: func() *int { v := 0; return &v }(),
			expectedPort:  0,
			description:   "When user explicitly sets 0, use 0 for random port",
		},
		{
			name:          "explicit custom port",
			wireguardPort: func() *int { v := 52000; return &v }(),
			expectedPort:  52000,
			description:   "When user sets custom port, use that port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.json")

			config, err := UpdateOrCreateConfig(ConfigInput{
				ConfigPath:    configPath,
				WireguardPort: tt.wireguardPort,
			})
			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectedPort, config.WgPort, tt.description)
		})
	}
}

func TestUpdateOldManagementURL(t *testing.T) {
	origProber := newMgmProber
	newMgmProber = func(_ context.Context, _ string, _ wgtypes.Key, _ bool) (mgmProber, error) {
		return &mockMgmProber{}, nil
	}
	t.Cleanup(func() { newMgmProber = origProber })

	tests := []struct {
		name                  string
		previousManagementURL string
		expectedManagementURL string
		fileShouldNotChange   bool
	}{
		{
			name:                  "Update old management URL with legacy port",
			previousManagementURL: "https://api.wiretrustee.com:33073",
			expectedManagementURL: DefaultManagementURL,
		},
		{
			name:                  "Update old management URL",
			previousManagementURL: oldDefaultManagementURL,
			expectedManagementURL: DefaultManagementURL,
		},
		{
			name:                  "No update needed when management URL is up to date",
			previousManagementURL: DefaultManagementURL,
			expectedManagementURL: DefaultManagementURL,
			fileShouldNotChange:   true,
		},
		{
			name:                  "No update needed when not using cloud management",
			previousManagementURL: "https://netbird.example.com:33073",
			expectedManagementURL: "https://netbird.example.com:33073",
			fileShouldNotChange:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.json")
			config, err := UpdateOrCreateConfig(ConfigInput{
				ManagementURL: tt.previousManagementURL,
				ConfigPath:    configPath,
			})
			require.NoError(t, err, "failed to create testing config")
			previousContent, err := os.ReadFile(configPath)
			require.NoError(t, err, "failed to read initial config")
			resultConfig, err := UpdateOldManagementURL(context.TODO(), config, configPath)
			require.NoError(t, err, "got error when updating old management url")
			require.Equal(t, tt.expectedManagementURL, resultConfig.ManagementURL.String())
			newContent, err := os.ReadFile(configPath)
			require.NoError(t, err, "failed to read updated config")
			if tt.fileShouldNotChange {
				require.Equal(t, string(previousContent), string(newContent), "file should not change")
			} else {
				require.NotEqual(t, string(previousContent), string(newContent), "file should have changed")
			}
		})
	}
}

func TestUpdateOldManagementURLSkipsAnonymousMode(t *testing.T) {
	proberCalled := false
	origProber := newMgmProber
	newMgmProber = func(_ context.Context, _ string, _ wgtypes.Key, _ bool) (mgmProber, error) {
		proberCalled = true
		return &mockMgmProber{}, nil
	}
	t.Cleanup(func() { newMgmProber = origProber })

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	config, err := UpdateOrCreateConfig(ConfigInput{
		ManagementURL: oldDefaultManagementURL,
		ConfigPath:    configPath,
	})
	require.NoError(t, err)
	config.AnonymousMode = true

	resultConfig, err := UpdateOldManagementURL(context.TODO(), config, configPath)
	require.NoError(t, err)
	require.Same(t, config, resultConfig)
	require.False(t, proberCalled, "anonymous mode must not probe cloud management URL")
	require.Equal(t, oldDefaultManagementURL, resultConfig.ManagementURL.String())
}
