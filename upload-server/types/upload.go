package types

import (
	"os"
	"strings"
)

const (
	// ClientHeader is the header used to identify the client
	ClientHeader = "x-nb-client"
	// ClientHeaderValue is the value of the ClientHeader
	ClientHeaderValue = "netbird"
	// GetURLPath is the path for the GetURL request
	GetURLPath = "/upload-url"

	DefaultBundleURLEnv = "ANONBIRD_DEBUG_UPLOAD_URL"
)

var DefaultBundleURL = strings.TrimSpace(os.Getenv(DefaultBundleURLEnv))

// GetURLResponse is the response for the GetURL request
type GetURLResponse struct {
	URL string
	Key string
}
