package anonymous

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureTorDaemonUsesExistingSOCKS(t *testing.T) {
	socksAddress := startFakeSOCKS5(t)

	daemon, err := EnsureTorDaemon(context.Background(), TransportConfig{
		Type:      TransportTorRelayOnly,
		TorSOCKS5: socksAddress,
	})

	require.NoError(t, err)
	require.NotNil(t, daemon)
	require.False(t, daemon.Started())
}

func TestCheckSOCKS5RejectsNonSOCKSListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("nope"))
	}()

	err = CheckSOCKS5(context.Background(), listener.Addr().String())
	require.Error(t, err)
}

func TestWriteTorConfig(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "with spaces")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))

	require.NoError(t, writeTorConfig(dataDir, "127.0.0.1:19050"))

	torrcBytes, err := os.ReadFile(filepath.Join(dataDir, "torrc"))
	require.NoError(t, err)
	torrc := string(torrcBytes)
	require.Contains(t, torrc, "DataDirectory "+torrcQuote(dataDir))
	require.Contains(t, torrc, "SocksPort 127.0.0.1:19050")
	require.Contains(t, torrc, "ClientOnly 1")
	require.Contains(t, torrc, "SafeSocks 1")
	require.Contains(t, torrc, "Log notice file "+torrcQuote(filepath.Join(dataDir, "tor.log")))
}

func startFakeSOCKS5(t *testing.T) string {
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
				header := make([]byte, 3)
				if _, err := io.ReadFull(reader, header); err != nil {
					return
				}
				if header[0] == 0x05 && header[1] == 0x01 && header[2] == 0x00 {
					_, _ = conn.Write([]byte{0x05, 0x00})
				}
			}()
		}
	}()

	return listener.Addr().String()
}
