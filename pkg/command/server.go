package command

import (
	"context"
	"fmt"
	"github.com/IljaN/opencloud-sftp/pkg/server"

	"github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/IljaN/opencloud-sftp/pkg/config/parser"
	"github.com/IljaN/opencloud-sftp/pkg/logging"
	"github.com/oklog/run"
	"github.com/opencloud-eu/opencloud/pkg/config/configlog"
	"github.com/opencloud-eu/reva/v2/pkg/sharedconf"
	"github.com/urfave/cli/v2"
)

// Server is the entry point for the server command.
func Server(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:     "server",
		Usage:    fmt.Sprintf("start the %s service without runtime (unsupervised mode)", cfg.Service.Name),
		Category: "server",
		Before: func(c *cli.Context) error {
			return configlog.ReturnFatal(parser.ParseConfig(cfg))
		},
		Action: func(c *cli.Context) error {

			logger := logging.Configure(cfg.Service.Name, cfg.Log)

			gr := run.Group{}
			_, cancel := context.WithCancel(c.Context)

			defer cancel()

			gr.Add(func() error {
				// init reva shared config explicitly as the go-micro based ocdav does not use
				// the reva runtime. But we need e.g. the shared client settings to be initialized
				sc := map[string]interface{}{
					"jwt_secret":                cfg.TokenManager.JWTSecret,
					"gatewaysvc":                cfg.Reva.Address,
					"skip_user_groups_in_token": cfg.SkipUserGroupsInToken,
					"grpc_client_options":       cfg.Reva.GetGRPCClientConfig(),
				}

				if err := sharedconf.Decode(sc); err != nil {
					logger.Error().Err(err).Msg("error decoding shared config for opencloud-sftp")
				}

				cfg.GatewaySelector = "eu.opencloud.api.gateway"
				srv := server.NewSFTPServer(cfg, logger)
				err := srv.ListenAndServe()
				if err != nil {
					return err
				}

				return nil

			}, func(err error) {
				if err == nil {
					logger.Info().
						Str("transport", "http").
						Str("server", cfg.Service.Name).
						Msg("Shutting down server")
				} else {
					logger.Error().Err(err).
						Str("transport", "http").
						Str("server", cfg.Service.Name).
						Msg("Shutting down server")
				}

				cancel()
			})

			return gr.Run()
		},
	}
}
