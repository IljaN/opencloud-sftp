package config

import (
	"context"

	"github.com/opencloud-eu/opencloud/pkg/shared"
)

type Config struct {
	Commons *shared.Commons `yaml:"-"` // don't use this directly as configuration for a service
	Service Service         `yaml:"-"`
	Tracing *Tracing        `yaml:"tracing"`
	Log     *Log            `yaml:"log"`
	Debug   Debug           `yaml:"debug"`

	TokenManager *TokenManager `yaml:"token_manager"`
	Reva         *shared.Reva  `yaml:"reva"`

	SkipUserGroupsInToken bool `yaml:"skip_user_groups_in_token" env:"OCSFTP_SKIP_USER_GROUPS_IN_TOKEN" desc:"Disables the loading of user's group memberships from the reva access token." introductionVersion:"1.0.0"`

	// Insecure certificates allowed when making requests to the gateway
	Insecure bool `yaml:"insecure" env:"OC_INSECURE;OCSFTP_INSECURE" desc:"Allow insecure connections to the GATEWAY service." introductionVersion:"1.0.0"`
	// Timeout in seconds when making requests to the gateway
	Timeout int64 `yaml:"gateway_request_timeout" env:"OCSFTP_GATEWAY_REQUEST_TIMEOUT" desc:"Request timeout in seconds for requests from the oCDAV service to the GATEWAY service." introductionVersion:"1.0.0"`

	MachineAuthAPIKey string `yaml:"machine_auth_api_key" env:"OC_MACHINE_AUTH_API_KEY;OCSFTP_MACHINE_AUTH_API_KEY" desc:"Machine auth API key used to validate internal requests necessary for the access to resources from other services." introductionVersion:"1.0.0"`

	Context context.Context `yaml:"-"`
	Status  Status          `yaml:"-"`

	AllowPropfindDepthInfinity bool `yaml:"allow_propfind_depth_infinity" env:"OCSFTP_ALLOW_PROPFIND_DEPTH_INFINITY" desc:"Allow the use of depth infinity in PROPFINDS. When enabled, a propfind will traverse through all subfolders. If many subfolders are expected, depth infinity can cause heavy server load and/or delayed response times." introductionVersion:"1.0.0"`
	ServerCertPath             string
	GatewaySelector            string
}

type Log struct {
	Level  string `yaml:"level" env:"OC_LOG_LEVEL;OCSFTP_LOG_LEVEL" desc:"The log level. Valid values are: 'panic', 'fatal', 'error', 'warn', 'info', 'debug', 'trace'." introductionVersion:"1.0.0"`
	Pretty bool   `yaml:"pretty" env:"OC_LOG_PRETTY;OCSFTP_LOG_PRETTY" desc:"Activates pretty log output." introductionVersion:"1.0.0"`
	Color  bool   `yaml:"color" env:"OC_LOG_COLOR;OCSFTP_LOG_COLOR" desc:"Activates colorized log output." introductionVersion:"1.0.0"`
	File   string `yaml:"file" env:"OC_LOG_FILE;OCSFTP_LOG_FILE" desc:"The path to the log file. Activates logging to this file if set." introductionVersion:"1.0.0"`
}

type Service struct {
	Name string `yaml:"-"`
}

type Debug struct {
	Addr   string `yaml:"addr" env:"OCSFTP_DEBUG_ADDR" desc:"Bind address of the debug server, where metrics, health, config and debug endpoints will be exposed." introductionVersion:"1.0.0"`
	Token  string `yaml:"token" env:"OCSFTP_DEBUG_TOKEN" desc:"Token to secure the metrics endpoint." introductionVersion:"1.0.0"`
	Pprof  bool   `yaml:"pprof" env:"OCSFTP_DEBUG_PPROF" desc:"Enables pprof, which can be used for profiling." introductionVersion:"1.0.0"`
	Zpages bool   `yaml:"zpages" env:"OCSFTP_DEBUG_ZPAGES" desc:"Enables zpages, which can be used for collecting and viewing in-memory traces." introductionVersion:"1.0.0"`
}

// CORS defines the available cors configuration.

// Status holds the configurable values for the status.php
type Status struct {
	Version        string
	VersionString  string
	Product        string
	ProductName    string
	ProductVersion string
	Edition        string `yaml:"edition" env:"OC_EDITION;OCSFTP_EDITION" desc:"Edition of OpenCloud. Used for branding purposes." introductionVersion:"1.0.0"`
}
