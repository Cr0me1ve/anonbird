package cmd

const (
	defaultMgmtDataDir   = "/var/lib/anonbird/"
	defaultMgmtConfigDir = "/etc/anonbird"
	defaultLogDir        = "/var/log/anonbird"

	legacyNetbirdMgmtDataDir   = "/var/lib/netbird/"
	legacyNetbirdMgmtConfigDir = "/etc/netbird"
	legacyNetbirdLogDir        = "/var/log/netbird"

	legacyWiretrusteeMgmtDataDir   = "/var/lib/wiretrustee/"
	legacyWiretrusteeMgmtConfigDir = "/etc/wiretrustee"
	legacyWiretrusteeLogDir        = "/var/log/wiretrustee"

	defaultMgmtConfig        = defaultMgmtConfigDir + "/management.json"
	defaultLogFile           = defaultLogDir + "/management.log"
	legacyNetbirdConfig      = legacyNetbirdMgmtConfigDir + "/management.json"
	legacyNetbirdLogFile     = legacyNetbirdLogDir + "/management.log"
	legacyWiretrusteeConfig  = legacyWiretrusteeMgmtConfigDir + "/management.json"
	legacyWiretrusteeLogFile = legacyWiretrusteeLogDir + "/management.log"

	defaultSingleAccModeDomain = "anonbird.selfhosted"
)
