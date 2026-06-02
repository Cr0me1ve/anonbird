//go:build !devartifactsign && (windows || darwin)

package installer

const (
	defaultSigningKeysBaseURL = ""
)
