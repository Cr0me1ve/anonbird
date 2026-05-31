package installer

import (
	"fmt"
	"os"
	"strings"
)

const (
	ReleaseBaseURLEnv       = "ANONBIRD_RELEASE_BASE_URL"
	SigningKeysBaseURLEnv   = "ANONBIRD_SIGNING_KEYS_BASE_URL"
	HomebrewFormulaEnv      = "ANONBIRD_HOMEBREW_FORMULA"
	HomebrewUIFormulaEnv    = "ANONBIRD_HOMEBREW_UI_FORMULA"
	HomebrewTapPathEnv      = "ANONBIRD_HOMEBREW_TAP_PATH"
	defaultReleaseBaseURL   = "https://github.com/Cr0me1ve/netbird/releases/download"
	defaultHomebrewTapOwner = "anonbird/homebrew-tap"
)

func releaseBaseURL() string {
	if value := strings.TrimSpace(os.Getenv(ReleaseBaseURLEnv)); value != "" {
		return strings.TrimRight(value, "/")
	}
	return defaultReleaseBaseURL
}

func signingKeysBaseURL() (string, error) {
	if value := strings.TrimSpace(os.Getenv(SigningKeysBaseURLEnv)); value != "" {
		return strings.TrimRight(value, "/"), nil
	}
	if defaultSigningKeysBaseURL != "" {
		return defaultSigningKeysBaseURL, nil
	}
	return "", fmt.Errorf("AnonBird auto-update signing keys URL is not configured; set %s", SigningKeysBaseURLEnv)
}

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
