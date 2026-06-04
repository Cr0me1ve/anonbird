package anonymous

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
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
		return &I2PDaemon{}, nil
	}
	if err := ValidateTransport(transport); err != nil {
		return nil, err
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := i2psam.Check(checkCtx, transport.I2PSAM)
	cancel()
	if err == nil {
		return &I2PDaemon{}, nil
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
	binaryPath, err := resolveI2PDaemonPath(ctx, transport.I2PDaemonPath)
	if err != nil {
		return nil, err
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err = i2psam.Check(checkCtx, transport.I2PSAM)
	cancel()
	if err == nil {
		return &I2PDaemon{}, nil
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
	if err := ensureI2PDataDirLayout(dataDir); err != nil {
		return nil, err
	}
	if err := writeI2PDConfig(dataDir, transport.I2PSAM); err != nil {
		return nil, err
	}

	logPath := filepath.Join(dataDir, "i2pd.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open i2pd log %s: %w", logPath, err)
	}
	if err := prepareI2PDataDirOwnership(dataDir); err != nil {
		_ = logFile.Close()
		return nil, err
	}

	pidFile, err := i2pPIDFile(dataDir)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}

	args := i2pdArgs(dataDir, pidFile)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if uid, gid, ok, err := packagedI2PUserIDsForRoot(); err != nil {
		_ = logFile.Close()
		return nil, err
	} else if ok {
		setI2PCommandCredential(cmd, uid, gid)
	}

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

func resolveI2PDaemonPath(ctx context.Context, path string) (string, error) {
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
	resolved, err := lookPath(path)
	if err != nil {
		if canAutoInstallBinary(path, DefaultI2PDaemonPath) {
			installed, installErr := ensurePackagedBinary(ctx, path, DefaultI2PDaemonPath, "i2pd")
			if installErr == nil {
				return installed, nil
			}
			return "", fmt.Errorf("i2pd binary %q not found in PATH and automatic install failed: %w", path, installErr)
		}
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

func i2pdArgs(dataDir, pidFile string) []string {
	return []string{
		"--datadir=" + dataDir,
		"--conf=" + filepath.Join(dataDir, "i2pd.conf"),
		"--tunconf=" + filepath.Join(dataDir, "tunnels.conf"),
		"--tunnelsdir=" + filepath.Join(dataDir, "tunnels.d"),
		"--pidfile=" + pidFile,
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

func ensureI2PDataDirLayout(dataDir string) error {
	for _, dir := range []string{"tunnels.d", "destinations"} {
		path := filepath.Join(dataDir, dir)
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create i2pd %s dir %s: %w", dir, path, err)
		}
	}
	return nil
}

func prepareI2PDataDirOwnership(dataDir string) error {
	uid, gid, ok, err := packagedI2PUserIDsForRoot()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if parent := i2pPackagedHomeParent(dataDir); parent != "" {
		if err := os.Chown(parent, uid, gid); err != nil {
			return fmt.Errorf("chown %s to i2pd: %w", parent, err)
		}
	}

	if err := filepath.WalkDir(dataDir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if chownErr := os.Chown(path, uid, gid); chownErr != nil {
			return fmt.Errorf("chown %s to i2pd: %w", path, chownErr)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("prepare i2pd data dir ownership: %w", err)
	}
	return nil
}

func packagedI2PUserIDsForRoot() (int, int, bool, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return 0, 0, false, nil
	}

	i2pUser, err := user.Lookup("i2pd")
	if err != nil {
		if _, ok := err.(user.UnknownUserError); ok {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("lookup i2pd user: %w", err)
	}
	uid, err := strconv.Atoi(i2pUser.Uid)
	if err != nil {
		return 0, 0, false, fmt.Errorf("parse i2pd uid %q: %w", i2pUser.Uid, err)
	}
	gid, err := strconv.Atoi(i2pUser.Gid)
	if err != nil {
		return 0, 0, false, fmt.Errorf("parse i2pd gid %q: %w", i2pUser.Gid, err)
	}
	return uid, gid, true, nil
}

func i2pPIDFile(dataDir string) (string, error) {
	uid, gid, ok, err := packagedI2PUserIDsForRoot()
	if err != nil {
		return "", err
	}
	if !ok {
		return filepath.Join(dataDir, "i2pd.pid"), nil
	}

	runtimeDir := filepath.Join(string(os.PathSeparator), "run", "i2pd")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", fmt.Errorf("create i2pd runtime dir %s: %w", runtimeDir, err)
	}
	if err := os.Chown(runtimeDir, uid, gid); err != nil {
		return "", fmt.Errorf("chown %s to i2pd: %w", runtimeDir, err)
	}
	pidFile := filepath.Join(runtimeDir, "i2pd.pid")
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale i2pd pid file %s: %w", pidFile, err)
	}
	return pidFile, nil
}

func i2pPackagedHomeParent(dataDir string) string {
	parent := filepath.Clean(filepath.Dir(dataDir))
	if parent == filepath.Join(string(os.PathSeparator), "var", "lib", "i2pd") {
		return parent
	}
	return ""
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
