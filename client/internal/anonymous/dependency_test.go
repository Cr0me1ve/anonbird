package anonymous

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsurePackagedBinaryInstallsDefaultPackage(t *testing.T) {
	restore := overrideDependencyHooks(t)
	defer restore()
	manager, expectedCommands := expectedPackageInstallCommands(t, "tor")

	installed := false
	lookPath = func(name string) (string, error) {
		switch name {
		case "tor":
			if installed {
				return "/usr/bin/tor", nil
			}
			return "", exec.ErrNotFound
		case manager:
			return "/usr/bin/" + manager, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	var commands []string
	runInstallStep = func(_ context.Context, name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if name == manager && isPackageInstallCommand(args) {
			installed = true
		}
		return nil
	}

	path, err := ensurePackagedBinary(context.Background(), "", DefaultTorDaemonPath, "tor")
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/tor", path)
	require.Equal(t, expectedCommands, commands)
}

func TestEnsurePackagedBinaryDoesNotInstallExplicitPath(t *testing.T) {
	restore := overrideDependencyHooks(t)
	defer restore()

	lookPath = func(_ string) (string, error) {
		return "", exec.ErrNotFound
	}
	runInstallStep = func(context.Context, string, ...string) error {
		t.Fatal("installer must not run for explicit binary paths")
		return nil
	}

	_, err := ensurePackagedBinary(context.Background(), "/tmp/missing-tor", DefaultTorDaemonPath, "tor")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing-tor")
}

func TestInstallSystemPackageFallsBackToSudo(t *testing.T) {
	restore := overrideDependencyHooks(t)
	defer restore()
	manager, expectedCommands := expectedPackageInstallCommands(t, "i2pd")
	if !managerAllowsSudo(manager) {
		t.Skip("package manager does not use sudo fallback")
	}
	expectedSudoCommands := append([]string{expectedCommands[0]}, sudoCommands(expectedCommands)...)

	lookPath = func(name string) (string, error) {
		switch name {
		case manager:
			return "/usr/bin/" + manager, nil
		case "sudo":
			return "/usr/bin/sudo", nil
		default:
			return "", exec.ErrNotFound
		}
	}

	var commands []string
	runInstallStep = func(_ context.Context, name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if name == manager {
			return errors.New("permission denied")
		}
		return nil
	}

	require.NoError(t, installSystemPackage(context.Background(), "i2pd"))
	require.Equal(t, expectedSudoCommands, commands)
}

func TestInstallSystemPackageAptInstallFallbackWhenUpdateFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("apt-get fallback is linux-specific")
	}

	restore := overrideDependencyHooks(t)
	defer restore()

	lookPath = func(name string) (string, error) {
		switch name {
		case "apt-get":
			return "/usr/bin/apt-get", nil
		default:
			return "", exec.ErrNotFound
		}
	}

	var commands []string
	runInstallStep = func(_ context.Context, name string, args ...string) error {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if len(args) > 0 && args[0] == "update" {
			return errors.New("third-party repository failed")
		}
		return nil
	}

	require.NoError(t, installSystemPackage(context.Background(), "tor"))
	require.Equal(t, []string{
		"apt-get update",
		"apt-get install -y tor",
	}, commands)
}

func overrideDependencyHooks(t *testing.T) func() {
	t.Helper()

	oldLookPath := lookPath
	oldRunInstallStep := runInstallStep
	return func() {
		lookPath = oldLookPath
		runInstallStep = oldRunInstallStep
	}
}

func expectedPackageInstallCommands(t *testing.T, packageName string) (string, []string) {
	t.Helper()

	candidates := packageManagersForOS(packageName)
	if len(candidates) == 0 {
		t.Skip("no package manager candidates for this OS")
	}
	candidate := candidates[0]
	commands := make([]string, 0, len(candidate.steps))
	for _, step := range candidate.steps {
		commands = append(commands, step.name+" "+strings.Join(step.args, " "))
	}
	return candidate.name, commands
}

func sudoCommands(commands []string) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		out = append(out, "/usr/bin/sudo -n "+command)
	}
	return out
}

func isPackageInstallCommand(args []string) bool {
	for _, arg := range args {
		if arg == "install" || arg == "add" {
			return true
		}
	}
	return len(args) > 0 && args[0] == "-Sy"
}
