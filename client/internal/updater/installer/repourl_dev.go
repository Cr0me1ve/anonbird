//go:build devartifactsign && (windows || darwin)

package installer

const (
	defaultSigningKeysBaseURL = "http://192.168.0.10:9089/signrepo"
)
