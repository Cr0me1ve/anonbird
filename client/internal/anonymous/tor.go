package anonymous

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	defaultTorStartTimeout = 2 * time.Minute
	defaultTorStopTimeout  = 5 * time.Second
)

type TorDaemon struct {
	cmd     *exec.Cmd
	done    chan error
	dataDir string
	logPath string
	started bool
}

func (d *TorDaemon) Started() bool {
	return d != nil && d.started
}

func (d *TorDaemon) DataDir() string {
	if d == nil {
		return ""
	}
	return d.dataDir
}

func (d *TorDaemon) Close() error {
	if d == nil || d.cmd == nil || d.cmd.Process == nil {
		return nil
	}

	if runtime.GOOS != "windows" {
		_ = d.cmd.Process.Signal(os.Interrupt)
	}

	select {
	case err := <-d.done:
		return err
	case <-time.After(defaultTorStopTimeout):
		if err := d.cmd.Process.Kill(); err != nil {
			return err
		}
		return <-d.done
	}
}

func EnsureTorDaemon(ctx context.Context, transport TransportConfig) (*TorDaemon, error) {
	transport = NormalizeTransport(transport)
	if transport.Type != TransportTorRelayOnly {
		return nil, nil
	}
	if err := ValidateTransport(transport); err != nil {
		return nil, err
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := CheckSOCKS5(checkCtx, transport.TorSOCKS5)
	cancel()
	if err == nil {
		return nil, nil
	}

	daemon, startErr := startTorDaemon(ctx, transport)
	if startErr != nil {
		return nil, fmt.Errorf("Tor SOCKS5 proxy %s is unavailable and auto-start failed: %w", transport.TorSOCKS5, startErr)
	}
	return daemon, nil
}

func CheckSOCKS5(ctx context.Context, address string) error {
	if err := validateHostPort("tor socks5", address); err != nil {
		return err
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 0x05 || resp[1] == 0xff {
		return fmt.Errorf("unexpected SOCKS5 handshake response %x", resp)
	}
	return nil
}

func startTorDaemon(ctx context.Context, transport TransportConfig) (*TorDaemon, error) {
	binaryPath, err := ensurePackagedBinary(ctx, DefaultTorDaemonPath, DefaultTorDaemonPath, "tor")
	if err != nil {
		return nil, fmt.Errorf("tor binary: %w", err)
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err = CheckSOCKS5(checkCtx, transport.TorSOCKS5)
	cancel()
	if err == nil {
		return nil, nil
	}

	dataDir, err := defaultTorDataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create Tor data dir %s: %w", dataDir, err)
	}
	if err := writeTorConfig(dataDir, transport.TorSOCKS5); err != nil {
		return nil, err
	}

	logPath := filepath.Join(dataDir, "tor.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Tor log %s: %w", logPath, err)
	}

	args := []string{"-f", filepath.Join(dataDir, "torrc")}
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start Tor %s: %w", binaryPath, err)
	}

	daemon := &TorDaemon{
		cmd:     cmd,
		done:    make(chan error, 1),
		dataDir: dataDir,
		logPath: logPath,
		started: true,
	}
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		daemon.done <- err
		close(daemon.done)
	}()

	if err := waitForTor(ctx, transport.TorSOCKS5, daemon); err != nil {
		_ = daemon.Close()
		return nil, err
	}

	log.Infof("managed Tor started with SOCKS5 proxy %s and data dir %s", transport.TorSOCKS5, dataDir)
	return daemon, nil
}

func waitForTor(ctx context.Context, socksAddress string, daemon *TorDaemon) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTorStartTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var socksReady bool
	for {
		checkCtx, checkCancel := context.WithTimeout(timeoutCtx, 2*time.Second)
		err := CheckSOCKS5(checkCtx, socksAddress)
		checkCancel()
		if err == nil {
			socksReady = true
			if torLogBootstrapped(daemon.logPath) {
				return nil
			}
		}

		select {
		case waitErr, ok := <-daemon.done:
			if !ok {
				waitErr = errors.New("Tor exited")
			}
			if socksReady {
				return fmt.Errorf("Tor exited before bootstrap completed (log: %s): %w", daemon.logPath, waitErr)
			}
			return fmt.Errorf("Tor exited before SOCKS5 proxy became ready (log: %s): %w", daemon.logPath, waitErr)
		case <-timeoutCtx.Done():
			if socksReady {
				return fmt.Errorf("timed out waiting for Tor bootstrap at %s (log: %s): %w", socksAddress, daemon.logPath, timeoutCtx.Err())
			}
			return fmt.Errorf("timed out waiting for Tor SOCKS5 proxy %s (log: %s): %w", socksAddress, daemon.logPath, timeoutCtx.Err())
		case <-ticker.C:
		}
	}
}

func torLogBootstrapped(logPath string) bool {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "Bootstrapped 100%")
}

func writeTorConfig(dataDir, socksAddress string) error {
	host, port, err := net.SplitHostPort(socksAddress)
	if err != nil {
		return fmt.Errorf("parse Tor SOCKS5 address %q: %w", socksAddress, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("parse Tor SOCKS5 port %q: %w", port, err)
	}
	host = normalizeLoopbackHost(host)

	logPath := filepath.Join(dataDir, "tor.log")
	torrc := strings.Join([]string{
		"DataDirectory " + torrcQuote(dataDir),
		"SocksPort " + net.JoinHostPort(host, port),
		"ClientOnly 1",
		"SafeSocks 1",
		"TestSocks 1",
		"ControlPort 0",
		"DNSPort 0",
		"TransPort 0",
		"RunAsDaemon 0",
		"Log notice file " + torrcQuote(logPath),
		"",
	}, "\n")

	if err := os.WriteFile(filepath.Join(dataDir, "torrc"), []byte(torrc), 0o600); err != nil {
		return fmt.Errorf("write Tor config: %w", err)
	}
	return nil
}

func torrcQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func normalizeLoopbackHost(host string) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" || strings.EqualFold(host, "localhost") {
		return "127.0.0.1"
	}
	return host
}

func defaultTorDataDir() (string, error) {
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		return filepath.Join(string(os.PathSeparator), "var", "lib", "anonbird", "tor"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(configDir, "anonbird", "tor"), nil
}
