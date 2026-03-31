package cli

import (
	"fmt"
	"os/exec"

	devcontainerbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newCodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "code <name>",
		Short: "Open VS Code attached to an instance",
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

			dc := devcontainerbackend.New()
			uri, ok := dc.EditorURI(inst.WorkspaceDir, target.Fleet)
			if !ok {
				return fmt.Errorf("editor integration not supported by this backend")
			}

			fmt.Printf("Opening VS Code for %s/%s...\n", target.Fleet, target.Instance)
			vscode := exec.Command("code", "--folder-uri", uri)
			return vscode.Run()
		},
	}
}
