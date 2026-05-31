package version

import (
	"os"
	"strings"
)

const (
	ProjectURLEnv  = "ANONBIRD_PROJECT_URL"
	DownloadURLEnv = "ANONBIRD_DOWNLOAD_URL"

	defaultProjectURL  = "https://github.com/Cr0me1ve/netbird"
	defaultDownloadURL = defaultProjectURL + "/releases"
)

var (
	projectURL  = configuredURL(ProjectURLEnv, defaultProjectURL)
	downloadURL = configuredURL(DownloadURLEnv, defaultDownloadURL)
)

func configuredURL(envName, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value
	}
	return fallback
}

func ProjectURL() string {
	return projectURL
}

func ReleasePageURL() string {
	return downloadURL
}
