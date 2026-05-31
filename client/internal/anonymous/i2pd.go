package anonymous

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/shared/anonymous/i2psam"
)

const (
	defaultI2PDaemonStartTimeout = 45 * time.Second
	defaultI2PDaemonStopTimeout  = 5 * time.Second
)

type I2PDaemon struct {
	cmd     *exec.Cmd
	done    chan error
	dataDir string
	logPath string
	started bool
}

func (d *I2PDaemon) Started() bool {
	return d != nil && d.started
}

func (d *I2PDaemon) DataDir() string {
	if d == nil {
		return ""
	}
	return d.dataDir
}

func (d *I2PDaemon) Close() error {
	if d == nil || d.cmd == nil || d.cmd.Process == nil {
		return nil
	}

	if runtime.GOOS != "windows" {
		_ = d.cmd.Process.Signal(os.Interrupt)
	}

	select {
	case err := <-d.done:
		return err
	case <-time.After(defaultI2PDaemonStopTimeout):
		if err := d.cmd.Process.Kill(); err != nil {
			return err
		}
		return <-d.done
	}
}

func EnsureI2PDaemon(ctx context.Context, transport TransportConfig) (*I2PDaemon, error) {
	transport = NormalizeTransport(transport)
	if transport.Type != TransportI2PDatagram {
		return nil, nil
	}
	if err := ValidateTransport(transport); err != nil {
		return nil, err
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := i2psam.Check(checkCtx, transport.I2PSAM)
	cancel()
	if err == nil {
		return nil, nil
	}
	if transport.I2PDaemonMode == I2PDaemonExternal {
		return nil, fmt.Errorf("check external i2p SAM bridge %s: %w", transport.I2PSAM, err)
	}

	daemon, startErr := startI2PDaemon(ctx, transport)
	if startErr != nil {
		if transport.I2PDaemonMode == I2PDaemonAuto {
			return nil, fmt.Errorf("i2p SAM bridge %s is unavailable and auto-start failed: %w", transport.I2PSAM, startErr)
		}
		return nil, startErr
	}
	return daemon, nil
}

func startI2PDaemon(ctx context.Context, transport TransportConfig) (*I2PDaemon, error) {
	binaryPath, err := resolveI2PDaemonPath(transport.I2PDaemonPath)
	if err != nil {
		return nil, err
	}

	dataDir := transport.I2PDataDir
	if dataDir == "" {
		dataDir, err = defaultI2PDataDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create i2pd data dir %s: %w", dataDir, err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tunnels.d"), 0o700); err != nil {
		return nil, fmt.Errorf("create i2pd tunnels dir %s: %w", dataDir, err)
	}
	if err := writeI2PDConfig(dataDir, transport.I2PSAM); err != nil {
		return nil, err
	}

	logPath := filepath.Join(dataDir, "i2pd.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open i2pd log %s: %w", logPath, err)
	}

	args := i2pdArgs(dataDir)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start i2pd %s: %w", binaryPath, err)
	}

	daemon := &I2PDaemon{
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

	if err := waitForI2PSAM(ctx, transport.I2PSAM, daemon); err != nil {
		_ = daemon.Close()
		return nil, err
	}

	log.Infof("managed i2pd started with SAM bridge %s and data dir %s", transport.I2PSAM, dataDir)
	return daemon, nil
}

func resolveI2PDaemonPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultI2PDaemonPath
	}
	if strings.ContainsRune(path, os.PathSeparator) || filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("i2pd binary %s is not available: %w", path, err)
		}
		return path, nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("i2pd binary %q not found in PATH", path)
	}
	return resolved, nil
}

func waitForI2PSAM(ctx context.Context, samAddress string, daemon *I2PDaemon) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultI2PDaemonStartTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		checkCtx, checkCancel := context.WithTimeout(timeoutCtx, 2*time.Second)
		err := i2psam.Check(checkCtx, samAddress)
		checkCancel()
		if err == nil {
			return nil
		}

		select {
		case waitErr, ok := <-daemon.done:
			if !ok {
				waitErr = errors.New("i2pd exited")
			}
			return fmt.Errorf("i2pd exited before SAM bridge became ready (log: %s): %w", daemon.logPath, waitErr)
		case <-timeoutCtx.Done():
			return fmt.Errorf("timed out waiting for i2pd SAM bridge %s (log: %s): %w", samAddress, daemon.logPath, timeoutCtx.Err())
		case <-ticker.C:
		}
	}
}

func i2pdArgs(dataDir string) []string {
	return []string{
		"--datadir=" + dataDir,
		"--conf=" + filepath.Join(dataDir, "i2pd.conf"),
		"--tunconf=" + filepath.Join(dataDir, "tunnels.conf"),
		"--tunnelsdir=" + filepath.Join(dataDir, "tunnels.d"),
		"--pidfile=" + filepath.Join(dataDir, "i2pd.pid"),
	}
}

func writeI2PDConfig(dataDir, samAddress string) error {
	host, port, err := net.SplitHostPort(samAddress)
	if err != nil {
		return fmt.Errorf("parse i2p SAM address %q: %w", samAddress, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("parse i2p SAM port %q: %w", port, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}

	conf := strings.Join([]string{
		"log = file",
		"loglevel = warn",
		"logfile = " + filepath.Join(dataDir, "i2pd.log"),
		"notransit = true",
		"upnp.enabled = false",
		"",
		"[http]",
		"enabled = false",
		"",
		"[httpproxy]",
		"enabled = false",
		"",
		"[socksproxy]",
		"enabled = false",
		"",
		"[i2cp]",
		"enabled = false",
		"",
		"[sam]",
		"enabled = true",
		"address = " + host,
		"port = " + port,
		"",
	}, "\n")

	if err := os.WriteFile(filepath.Join(dataDir, "i2pd.conf"), []byte(conf), 0o600); err != nil {
		return fmt.Errorf("write i2pd config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "tunnels.conf"), []byte("# AnonBird uses SAM sessions; static tunnels are not required.\n"), 0o600); err != nil {
		return fmt.Errorf("write i2pd tunnels config: %w", err)
	}
	return nil
}

func defaultI2PDataDir() (string, error) {
	if runtime.GOOS == "linux" {
		if os.Geteuid() == 0 {
			return filepath.Join(string(os.PathSeparator), "var", "lib", "i2pd", "anonbird"), nil
		}
		homeDir, err := os.UserHomeDir()
		if err == nil && homeDir != "" {
			return filepath.Join(homeDir, ".i2pd", "anonbird"), nil
		}
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(configDir, "anonbird", "i2pd"), nil
}
