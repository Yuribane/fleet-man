package cli

import (
	"fmt"
	"os"

	devcontainerbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy <fleet>",
		Short: "Remove a fleet and all its instances",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fleetName := args[0]

			st, err := state.Load()
			if err != nil {
				return err
			}

			f, ok := st.Fleets[fleetName]
			if !ok {
				return fmt.Errorf("fleet %q not found", fleetName)
			}

			dc := devcontainerbackend.New()
			for _, inst := range f.Instances {
				fmt.Printf("Stopping %s/%s...\n", fleetName, inst.Name)
				if err := dc.Down(inst.ContainerID); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to remove container for %s: %v\n", inst.Name, err)
				}
				if inst.WorkspaceDir != "" {
					if err := os.RemoveAll(inst.WorkspaceDir); err != nil {
						fmt.Fprintf(os.Stderr, "warning: failed to remove workspace for %s: %v\n", inst.Name, err)
					}
				}
			}

			delete(st.Fleets, fleetName)

			if err := state.Save(st); err != nil {
				return err
			}

			fmt.Printf("Fleet %s destroyed (%d instances removed).\n", fleetName, len(f.Instances))
			return nil
		},
	}
}
