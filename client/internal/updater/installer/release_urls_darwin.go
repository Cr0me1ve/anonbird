//go:build darwin

package installer

import (
	"fmt"
	"os"
	"strings"
)

const (
	HomebrewFormulaEnv      = "ANONBIRD_HOMEBREW_FORMULA"
	HomebrewUIFormulaEnv    = "ANONBIRD_HOMEBREW_UI_FORMULA"
	HomebrewTapPathEnv      = "ANONBIRD_HOMEBREW_TAP_PATH"
	defaultHomebrewTapOwner = "Cr0me1ve/homebrew-anonbird"
)

func configuredHomebrewFormula() (string, error) {
	formula := strings.TrimSpace(os.Getenv(HomebrewFormulaEnv))
	if formula == "" {
		return "", fmt.Errorf("AnonBird Homebrew auto-update is not configured; set %s", HomebrewFormulaEnv)
	}
	return formula, nil
}

func configuredHomebrewUIFormula() string {
	return strings.TrimSpace(os.Getenv(HomebrewUIFormulaEnv))
}

func configuredHomebrewTapPaths() []string {
	if value := strings.TrimSpace(os.Getenv(HomebrewTapPathEnv)); value != "" {
		return []string{value}
	}
	return []string{
		"/opt/homebrew/Library/Taps/" + defaultHomebrewTapOwner + "/",
		"/usr/local/Homebrew/Library/Taps/" + defaultHomebrewTapOwner + "/",
	}
}
