package parser

import (
	"errors"
	"github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/IljaN/opencloud-sftp/pkg/config/defaults"
	occfg "github.com/opencloud-eu/opencloud/pkg/config"
	ocparse "github.com/opencloud-eu/opencloud/pkg/config/parser"

	"github.com/opencloud-eu/opencloud/pkg/shared"

	"github.com/opencloud-eu/opencloud/pkg/config/envdecode"
)

// ParseConfig loads configuration from known paths.
func ParseConfig(cfg *config.Config) error {
	globConf := occfg.DefaultConfig()
	err := ocparse.ParseConfig(globConf, true)
	cfg.Commons = globConf.Commons
	if err != nil {
		return err
	}

	err = occfg.BindSourcesToStructs(cfg.Service.Name, globConf)
	if err != nil {
		return err
	}

	defaults.EnsureDefaults(cfg)

	// load all env variables relevant to the config in the current context.
	if err := envdecode.Decode(cfg); err != nil {
		// no environment variable set for this config is an expected "error"
		if !errors.Is(err, envdecode.ErrNoTargetFieldsAreSet) {
			return err
		}
	}

	defaults.Sanitize(cfg)

	return Validate(cfg)
}

func Validate(cfg *config.Config) error {
	if cfg.TokenManager.JWTSecret == "" {
		return shared.MissingJWTTokenError(cfg.Service.Name)
	}
	if cfg.MachineAuthAPIKey == "" {
		return shared.MissingMachineAuthApiKeyError(cfg.Service.Name)
	}

	return nil
}
