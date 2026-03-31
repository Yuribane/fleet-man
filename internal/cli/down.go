package cli

import (
	"fmt"
	"os"

	devcontainerbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down <name>",
		Short: "Stop and remove a devcontainer instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := fleet.Resolve(args[0], "")
			if err != nil {
				return err
			}

			st, err := state.Load()
			if err != nil {
				return err
			}

			f, ok := st.Fleets[target.Fleet]
			if !ok {
				return fmt.Errorf("fleet %q not found", target.Fleet)
			}

			inst, err := f.GetInstance(target.Instance)
			if err != nil {
				return err
			}

			// Stop the container
			fmt.Printf("Stopping %s/%s...\n", target.Fleet, target.Instance)
			dc := devcontainerbackend.New()
			if err := dc.Down(inst.ContainerID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove container: %v\n", err)
			}

			// Remove the workspace directory
			if inst.WorkspaceDir != "" {
				if err := os.RemoveAll(inst.WorkspaceDir); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to remove workspace dir: %v\n", err)
				}
			}

			// Remove from state
			if err := f.RemoveInstance(target.Instance); err != nil {
				return err
			}

			if err := state.Save(st); err != nil {
				return err
			}

			fmt.Printf("Instance %s/%s removed.\n", target.Fleet, target.Instance)
			return nil
		},
	}
}
