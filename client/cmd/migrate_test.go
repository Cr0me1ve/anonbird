package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
	writeTestFile(t, filepath.Join(root, "etc/systemd/system/netbird.service"), `[Unit]
Description=NetBird
[Service]
ExecStart=/usr/bin/netbird service run --config /etc/netbird/config.json --log-file /var/log/netbird/client.log --daemon-addr unix:///var/run/netbird.sock
Environment=SYSTEMD_UNIT=netbird
`)

	opts := migrationOptions{
		Apply:     true,
		Root:      root,
		BackupDir: "/var/backups/anonbird/migration-test",
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
	require.Contains(t, string(unit), "SYSTEMD_UNIT=anonbird")

	require.FileExists(t, filepath.Join(root, "var/backups/anonbird/migration-test/manifest.json"))
	require.FileExists(t, filepath.Join(root, "var/backups/anonbird/migration-test/source/etc/netbird/config.json"))
}

func TestRollbackClientMigrationRestoresExistingTarget(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "etc/netbird/config.json"), "from netbird")
	writeTestFile(t, filepath.Join(root, "etc/anonbird/config.json"), "existing anonbird")

	opts := migrationOptions{
		Apply:     true,
		Root:      root,
		BackupDir: "/var/backups/anonbird/migration-test",
		Force:     true,
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
