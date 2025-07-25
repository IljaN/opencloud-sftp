package command

import (
	"fmt"
	"os"

	"github.com/opencloud-eu/opencloud/pkg/registry"
	"github.com/opencloud-eu/opencloud/pkg/version"

	"github.com/IljaN/opencloud-sftp/pkg/config"
	tw "github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

// Version prints the service versions of all running instances.
func Version(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:     "version",
		Usage:    "print the version of this binary and the running service instances",
		Category: "info",
		Action: func(c *cli.Context) error {
			fmt.Println("Version: " + version.GetString())
			fmt.Printf("Compiled: %s\n", version.Compiled())
			fmt.Println("")

			reg := registry.GetRegistry()
			services, err := reg.GetService("SFTP" + "." + cfg.Service.Name)
			if err != nil {
				fmt.Println(fmt.Errorf("could not get %s services from the registry: %v", cfg.Service.Name, err))
				return err
			}

			if len(services) == 0 {
				fmt.Println("No running " + cfg.Service.Name + " service found.")
				return nil
			}

			table := tw.NewWriter(os.Stdout)
			table.Header([]string{"Version", "Address", "Id"})
			for _, s := range services {
				for _, n := range s.Nodes {
					table.Append([]string{s.Version, n.Address, n.Id})
				}
			}
			table.Render()
			return nil
		},
	}
}
