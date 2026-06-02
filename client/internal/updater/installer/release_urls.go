//go:build windows || darwin

package installer

import (
	"fmt"
	"os"
	"strings"
)

const (
	ReleaseBaseURLEnv     = "ANONBIRD_RELEASE_BASE_URL"
	SigningKeysBaseURLEnv = "ANONBIRD_SIGNING_KEYS_BASE_URL"
	defaultReleaseBaseURL = "https://github.com/Cr0me1ve/anonbird/releases/download"
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
