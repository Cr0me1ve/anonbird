//go:build !windows && !ios && !android

package daemonaddr

import (
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

var scanDir = "/var/run/anonbird"

// setScanDir overrides the scan directory (used by tests).
func setScanDir(dir string) {
	scanDir = dir
}

// ResolveUnixDaemonAddr checks whether the default Unix socket exists and, if not,
// scans /var/run/anonbird/ for a single .sock file to use instead. This handles the
// mismatch between service templates that place the socket under
// /var/run/anonbird/<instance>.sock and the CLI default (/var/run/anonbird.sock).
func ResolveUnixDaemonAddr(addr string) string {
	if !strings.HasPrefix(addr, "unix://") {
		return addr
	}

	sockPath := strings.TrimPrefix(addr, "unix://")
	if _, err := os.Stat(sockPath); err == nil {
		return addr
	}

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		return addr
	}

	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".sock") {
			found = append(found, filepath.Join(scanDir, e.Name()))
		}
	}

	switch len(found) {
	case 1:
		resolved := "unix://" + found[0]
		log.Debugf("Default daemon socket not found, using discovered socket: %s", resolved)
		return resolved
	case 0:
		return addr
	default:
		log.Warnf("Default daemon socket not found and multiple sockets discovered in %s; pass --daemon-addr explicitly", scanDir)
		return addr
	}
}
