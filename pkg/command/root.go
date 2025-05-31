package command

import (
	"github.com/urfave/cli/v2"
	"os"

	"github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/opencloud-eu/opencloud/pkg/clihelper"
)

// GetCommands provides all commands for this service
func GetCommands(cfg *config.Config) cli.Commands {
	return []*cli.Command{
		Server(cfg),
		Health(cfg),
		Version(cfg),
	}
}

// Execute is the entry point for the OpenCloud ocdav command.
func Execute(cfg *config.Config) error {
	app := clihelper.DefaultApp(&cli.App{
		Name:     "sftp",
		Usage:    "Provide an SFTP interface for OpenCloud",
		Commands: GetCommands(cfg),
	})

	return app.RunContext(cfg.Context, os.Args)
}
