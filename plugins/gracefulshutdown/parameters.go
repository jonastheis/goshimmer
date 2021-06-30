package gracefulshutdown

import (
	"github.com/iotaledger/hive.go/configuration"
)

// ParametersDefinition contains the definition of configuration parameters used by the graceful shutdown plugin.
type ParametersDefinition struct {
	// WaitToKillTime is the maximum amount of time (in seconds) to wait for background processes to terminate.
	WaitToKillTime int `default:"120" usage:"the maximum amount of time (in seconds) to wait for background processes to terminate"`
}

// Parameters contains the configuration parameters of the graceful shutdown plugin.
var Parameters = ParametersDefinition{}

func init() {
	configuration.BindParameters(&Parameters, "gracefulshutdown")
}
