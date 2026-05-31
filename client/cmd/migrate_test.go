package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/profilemanager"
)

func TestBuildClientMigrationPlanDetectsLegacyPaths(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), `{"ManagementURL":"https://netbird.example"}`)
	writeTestFile(t, filepath.Join(root, "var/lib/netbird/service.json"), `{"service":"netbird"}`)
	writeTestFile(t, filepath.Join(root, "etc/systemd/system/netbird.service"), `[Service]
ExecStart=/usr/bin/netbird service run --config /etc/netbird/config.json --log-file /var/log/netbird/client.log --daemon-addr unix:///var/run/netbird.sock
Environment=SYSTEMD_UNIT=netbird
`)

	plan, err := buildClientMigrationPlan(migrationOptions{Root: root, BackupDir: "/var/backups/anonbird/migration-test"})
	require.NoError(t, err)
	require.Equal(t, "client", plan.Scope)
	require.Len(t, plan.Actions, 3)
	require.Equal(t, filepath.Join(root, "var/backups/anonbird/migration-test"), plan.BackupDir)
	require.Equal(t, "/etc/netbird", plan.Actions[0].Source)
	require.Equal(t, "/etc/anonbird", plan.Actions[0].Target)
	require.True(t, plan.Actions[2].Rewrite)
}

func TestApplyClientMigrationPlanCopiesAndRewrites(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), `{"ManagementURL":"https://netbird.example"}`)
	writeTestFile(t, filepath.Join(root, "var/lib/netbird/service.json"), `{"service":"netbird"}`)
	writeTestFile(t, filepath.Join(root, "var/log/netbird/client.log"), "old log")
	writeTestFile(t, filepath.Join(root, "etc/sysconfig/netbird"), "NB_CONFIG=/etc/netbird/config.json\nNB_LOG_FILE=/var/log/netbird/client.log\n")
	writeTestFile(t, filepath.Join(root, "etc/systemd/system/netbird.service"), `[Unit]
Description=NetBird
[Service]
ExecStart=/usr/bin/netbird service run --config /etc/netbird/config.json --log-file /var/log/netbird/client.log --daemon-addr unix:///var/run/netbird.sock
EnvironmentFile=-/etc/sysconfig/netbird
Environment=SYSTEMD_UNIT=netbird
`)

	opts := migrationOptions{
		Apply:               true,
		Root:                root,
		BackupDir:           "/var/backups/anonbird/migration-test",
		AllowUnsafeClearnet: true,
		UnsafeClearnetAck:   true,
	}
	plan, err := buildClientMigrationPlan(opts)
	require.NoError(t, err)
	require.NoError(t, applyClientMigrationPlan(noopWriter{}, plan, opts))

	require.FileExists(t, filepath.Join(root, "etc/anonbird/config.json"))
	require.FileExists(t, filepath.Join(root, "var/lib/anonbird/service.json"))
	require.FileExists(t, filepath.Join(root, "var/log/anonbird/client.log"))

	unit, err := os.ReadFile(filepath.Join(root, "etc/systemd/system/anonbird.service"))
	require.NoError(t, err)
	require.Contains(t, string(unit), "Description=AnonBird")
	require.Contains(t, string(unit), "/usr/bin/anonbird service run")
	require.Contains(t, string(unit), "/etc/anonbird/config.json")
	require.Contains(t, string(unit), "/var/log/anonbird/client.log")
	require.Contains(t, string(unit), "unix:///var/run/anonbird.sock")
	require.Contains(t, string(unit), "EnvironmentFile=-/etc/sysconfig/anonbird")
	require.Contains(t, string(unit), "SYSTEMD_UNIT=anonbird")

	envFile, err := os.ReadFile(filepath.Join(root, "etc/sysconfig/anonbird"))
	require.NoError(t, err)
	require.Contains(t, string(envFile), "NB_CONFIG=/etc/anonbird/config.json")
	require.Contains(t, string(envFile), "NB_LOG_FILE=/var/log/anonbird/client.log")

	require.FileExists(t, filepath.Join(root, "var/backups/anonbird/migration-test/manifest.json"))
	require.FileExists(t, filepath.Join(root, "var/backups/anonbird/migration-test/source/etc/netbird/config.json"))
}

func TestApplyClientMigrationRefusesUnsafeClearnetConfig(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), `{"ManagementURL":"https://netbird.example","anonymous_mode":false}`)

	opts := migrationOptions{
		Apply:     true,
		Root:      root,
		BackupDir: "/var/backups/anonbird/migration-test",
	}
	plan, err := buildClientMigrationPlan(opts)
	require.NoError(t, err)

	err = applyClientMigrationPlan(noopWriter{}, plan, opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--rejoin")
	require.Contains(t, err.Error(), "--allow-unsafe-clearnet")
	require.NoFileExists(t, filepath.Join(root, "etc/anonbird/config.json"))
}

func TestApplyClientMigrationWithRejoinHardensConfig(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), `{
  "ManagementURL": "https://netbird.example",
  "PrivateKey": "legacy-private"
}`)
	writeTestFile(t, filepath.Join(root, "etc/netbird/management-url"), "https://api.netbird.io\n")
	writeTestFile(t, filepath.Join(root, "etc/netbird/setup-key"), "OLD-SETUP-KEY\n")

	rejoin := "anonbird://join?server=http%3A%2F%2Fmanagementexampleabcdefghijklmnop.onion&setup_key=NB-SETUP-xxxx&transport=tor-relay-only&tor_socks5=127.0.0.1%3A9051"
	opts := migrationOptions{
		Apply:     true,
		Root:      root,
		BackupDir: "/var/backups/anonbird/migration-test",
		Rejoin:    rejoin,
	}
	plan, err := buildClientMigrationPlan(opts)
	require.NoError(t, err)
	require.NoError(t, applyClientMigrationPlan(noopWriter{}, plan, opts))

	data, err := os.ReadFile(filepath.Join(root, "etc/anonbird/config.json"))
	require.NoError(t, err)
	var migrated map[string]any
	require.NoError(t, json.Unmarshal(data, &migrated))
	managementURL, ok := migrated["ManagementURL"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "http", managementURL["Scheme"])
	require.Equal(t, "managementexampleabcdefghijklmnop.onion", managementURL["Host"])
	adminURL, ok := migrated["AdminURL"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "http", adminURL["Scheme"])
	require.Equal(t, "localhost:33071", adminURL["Host"])
	require.Equal(t, true, migrated["anonymous_mode"])
	require.Equal(t, true, migrated["DisableAutoConnect"])
	transport, ok := migrated["anonymous_transport"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tor-relay-only", transport["type"])
	require.Equal(t, "127.0.0.1:9051", transport["tor_socks5"])
	require.Equal(t, true, transport["require_anonymous"])
	require.Equal(t, "legacy-private", migrated["PrivateKey"])

	var config profilemanager.Config
	require.NoError(t, json.Unmarshal(data, &config))
	require.Equal(t, "http://managementexampleabcdefghijklmnop.onion", config.ManagementURL.String())
	require.True(t, config.AnonymousMode)
	require.True(t, config.DisableAutoConnect)

	require.Equal(t, "http://managementexampleabcdefghijklmnop.onion\n", readTestFile(t, filepath.Join(root, "etc/anonbird/management-url")))
	require.Equal(t, "NB-SETUP-xxxx\n", readTestFile(t, filepath.Join(root, "etc/anonbird/setup-key")))
	setupKeyInfo, err := os.Stat(filepath.Join(root, "etc/anonbird/setup-key"))
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), setupKeyInfo.Mode().Perm())
	}
}

func TestApplyClientMigrationAllowsUnsafeClearnetWithExplicitAck(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), `{"ManagementURL":"https://netbird.example","anonymous_mode":false}`)

	opts := migrationOptions{
		Apply:               true,
		Root:                root,
		BackupDir:           "/var/backups/anonbird/migration-test",
		AllowUnsafeClearnet: true,
		UnsafeClearnetAck:   true,
	}
	plan, err := buildClientMigrationPlan(opts)
	require.NoError(t, err)
	require.NoError(t, applyClientMigrationPlan(noopWriter{}, plan, opts))
	require.FileExists(t, filepath.Join(root, "etc/anonbird/config.json"))
}

func TestRollbackClientMigrationRestoresExistingTarget(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), "from netbird")
	writeTestFile(t, filepath.Join(root, "etc/anonbird/config.json"), "existing anonbird")

	opts := migrationOptions{
		Apply:               true,
		Root:                root,
		BackupDir:           "/var/backups/anonbird/migration-test",
		Force:               true,
		AllowUnsafeClearnet: true,
		UnsafeClearnetAck:   true,
	}
	plan, err := buildClientMigrationPlan(opts)
	require.NoError(t, err)
	require.NoError(t, applyClientMigrationPlan(noopWriter{}, plan, opts))
	require.Equal(t, "from netbird", readTestFile(t, filepath.Join(root, "etc/anonbird/config.json")))

	require.NoError(t, rollbackClientMigration(noopWriter{}, opts))
	require.Equal(t, "existing anonbird", readTestFile(t, filepath.Join(root, "etc/anonbird/config.json")))
}

func TestFindLatestClientMigrationBackupReturnsLogicalPath(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"migration-20260531-100000", "migration-20260531-110000"} {
		dir := filepath.Join(root, "var/backups/anonbird", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		data, err := json.Marshal(migrationManifest{Scope: "client"})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, migrationManifestName), data, 0o600))
	}

	backupDir, err := findLatestClientMigrationBackup(migrationOptions{Root: root})
	require.NoError(t, err)
	require.Equal(t, "/var/backups/anonbird/migration-20260531-110000", backupDir)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
