package anonymous

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"
)

type installStep struct {
	name string
	args []string
}

var (
	lookPath         = exec.LookPath
	runInstallStep   = defaultRunInstallStep
	installStepLimit = 4 * 1024
)

func resolveExecutablePath(path, defaultName string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultName
	}
	if strings.ContainsRune(path, os.PathSeparator) || filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("binary %s is not available: %w", path, err)
		}
		return path, nil
	}
	resolved, err := lookPath(path)
	if err != nil {
		return "", fmt.Errorf("binary %q not found in PATH", path)
	}
	return resolved, nil
}

func ensurePackagedBinary(ctx context.Context, path, defaultName, packageName string) (string, error) {
	resolved, err := resolveExecutablePath(path, defaultName)
	if err == nil {
		return resolved, nil
	}
	if !canAutoInstallBinary(path, defaultName) {
		return "", err
	}

	installErr := installSystemPackage(ctx, packageName)
	if installErr != nil {
		return "", fmt.Errorf("%s and automatic install of package %q failed: %w", err, packageName, installErr)
	}

	resolved, err = resolveExecutablePath(path, defaultName)
	if err != nil {
		return "", fmt.Errorf("package %q was installed but %s is still unavailable: %w", packageName, defaultName, err)
	}
	return resolved, nil
}

func canAutoInstallBinary(path, defaultName string) bool {
	path = strings.TrimSpace(path)
	return path == "" || path == defaultName
}

func installSystemPackage(ctx context.Context, packageName string) error {
	steps, manager, err := installSteps(packageName)
	if err != nil {
		return err
	}
	log.Infof("installing anonymous runtime dependency package %q with %s", packageName, manager)

	err = runInstallStepsWithOptionalSudo(ctx, manager, steps)
	if err == nil {
		return nil
	}

	if manager == "apt-get" {
		fallback := []installStep{{name: "apt-get", args: []string{"install", "-y", packageName}}}
		log.Warnf("apt-get update/install failed, retrying install without update: %v", err)
		if fallbackErr := runInstallStepsWithOptionalSudo(ctx, manager, fallback); fallbackErr == nil {
			return nil
		} else {
			return fmt.Errorf("%w; apt-get install fallback failed: %v", err, fallbackErr)
		}
	}

	return err
}

func managerAllowsSudo(manager string) bool {
	return manager != "brew"
}

func installSteps(packageName string) ([]installStep, string, error) {
	candidates := packageManagersForOS(packageName)
	for _, candidate := range candidates {
		if _, err := lookPath(candidate.name); err == nil {
			return candidate.steps, candidate.name, nil
		}
	}
	return nil, "", fmt.Errorf("no supported package manager found to install %q; install it manually or run AnonBird from a package that bundles anonymous runtime dependencies", packageName)
}

type packageManagerCandidate struct {
	name  string
	steps []installStep
}

func packageManagersForOS(packageName string) []packageManagerCandidate {
	switch runtime.GOOS {
	case "linux":
		return []packageManagerCandidate{
			{name: "apt-get", steps: []installStep{
				{name: "apt-get", args: []string{"update"}},
				{name: "apt-get", args: []string{"install", "-y", packageName}},
			}},
			{name: "dnf", steps: []installStep{{name: "dnf", args: []string{"install", "-y", packageName}}}},
			{name: "yum", steps: []installStep{{name: "yum", args: []string{"install", "-y", packageName}}}},
			{name: "zypper", steps: []installStep{{name: "zypper", args: []string{"--non-interactive", "install", packageName}}}},
			{name: "apk", steps: []installStep{{name: "apk", args: []string{"add", packageName}}}},
			{name: "pacman", steps: []installStep{{name: "pacman", args: []string{"-Sy", "--noconfirm", packageName}}}},
		}
	case "darwin":
		return []packageManagerCandidate{
			{name: "brew", steps: []installStep{{name: "brew", args: []string{"install", packageName}}}},
		}
	case "freebsd", "openbsd", "netbsd", "dragonfly":
		return []packageManagerCandidate{
			{name: "pkg", steps: []installStep{{name: "pkg", args: []string{"install", "-y", packageName}}}},
		}
	default:
		return nil
	}
}

func runInstallSteps(ctx context.Context, steps []installStep) error {
	for _, step := range steps {
		if err := runInstallStep(ctx, step.name, step.args...); err != nil {
			return err
		}
	}
	return nil
}

func runInstallStepsWithOptionalSudo(ctx context.Context, manager string, steps []installStep) error {
	if err := runInstallSteps(ctx, steps); err == nil {
		return nil
	} else if managerAllowsSudo(manager) {
		sudoPath, sudoErr := lookPath("sudo")
		if sudoErr != nil {
			return err
		}
		log.Debugf("package install with %s failed, retrying with sudo -n: %v", manager, err)
		return runInstallStepsWithSudo(ctx, sudoPath, steps)
	} else {
		return err
	}
}

func runInstallStepsWithSudo(ctx context.Context, sudoPath string, steps []installStep) error {
	for _, step := range steps {
		args := append([]string{"-n", step.name}, step.args...)
		if err := runInstallStep(ctx, sudoPath, args...); err != nil {
			return err
		}
	}
	return nil
}

func defaultRunInstallStep(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w%s", name, strings.Join(args, " "), err, formatCommandOutput(out))
	}
	return nil
}

func formatCommandOutput(out []byte) string {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > installStepLimit {
		trimmed = trimmed[len(trimmed)-installStepLimit:]
	}
	return ": " + trimmed
}
