package anonymous

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureI2PDaemonExternalUsesExistingSAM(t *testing.T) {
	samAddress := startFakeSAM(t)

	daemon, err := EnsureI2PDaemon(context.Background(), TransportConfig{
		Type:          TransportI2PDatagram,
		I2PSAM:        samAddress,
		I2PDaemonMode: I2PDaemonExternal,
	})

	require.NoError(t, err)
	require.NotNil(t, daemon)
	require.False(t, daemon.Started())
}

func TestEnsureI2PDaemonManagedRequiresBinary(t *testing.T) {
	_, err := EnsureI2PDaemon(context.Background(), TransportConfig{
		Type:          TransportI2PDatagram,
		I2PSAM:        freeLocalAddress(t),
		I2PDaemonMode: I2PDaemonManaged,
		I2PDaemonPath: filepath.Join(t.TempDir(), "missing-i2pd"),
		I2PDataDir:    t.TempDir(),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "i2pd binary")
}

func TestWriteI2PDConfig(t *testing.T) {
	dataDir := t.TempDir()

	require.NoError(t, writeI2PDConfig(dataDir, "127.0.0.1:17656"))

	confBytes, err := os.ReadFile(filepath.Join(dataDir, "i2pd.conf"))
	require.NoError(t, err)
	conf := string(confBytes)
	require.Contains(t, conf, "[sam]")
	require.Contains(t, conf, "enabled = true")
	require.Contains(t, conf, "address = 127.0.0.1")
	require.Contains(t, conf, "port = 17656")
	require.Contains(t, conf, "notransit = true")
}

func TestI2PDArgsUseManagedTunnelsDir(t *testing.T) {
	dataDir := t.TempDir()
	pidFile := filepath.Join(dataDir, "i2pd.pid")

	args := i2pdArgs(dataDir, pidFile)

	require.Contains(t, args, "--tunconf="+filepath.Join(dataDir, "tunnels.conf"))
	require.Contains(t, args, "--tunnelsdir="+filepath.Join(dataDir, "tunnels.d"))
	require.Contains(t, args, "--pidfile="+pidFile)
}

func TestEnsureI2PDataDirLayout(t *testing.T) {
	dataDir := t.TempDir()

	require.NoError(t, ensureI2PDataDirLayout(dataDir))
	require.DirExists(t, filepath.Join(dataDir, "tunnels.d"))
	require.DirExists(t, filepath.Join(dataDir, "destinations"))
}

func TestI2PPackagedHomeParent(t *testing.T) {
	dataDir := filepath.Join(string(os.PathSeparator), "var", "lib", "i2pd", "anonbird")

	require.Equal(t, filepath.Dir(dataDir), i2pPackagedHomeParent(dataDir))
	require.Empty(t, i2pPackagedHomeParent(filepath.Join(t.TempDir(), "anonbird")))
}

func startFakeSAM(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				line, err := reader.ReadString('\n')
				if err != nil || !strings.HasPrefix(line, "HELLO VERSION") {
					return
				}
				_, _ = conn.Write([]byte("HELLO REPLY RESULT=OK VERSION=3.3\n"))
			}()
		}
	}()

	return listener.Addr().String()
}

func freeLocalAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}
