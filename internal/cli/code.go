package cli

import (
	"encoding/hex"
	"fmt"
	"os/exec"

	"github.com/fleet-man/fleet-man/internal/fleet"
	"github.com/fleet-man/fleet-man/internal/state"
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

			// VS Code remote containers uses hex-encoded path
			hexPath := hex.EncodeToString([]byte(inst.WorkspaceDir))
			folderURI := fmt.Sprintf("vscode-remote://dev-container+%s/workspaces/%s", hexPath, target.Fleet)

			fmt.Printf("Opening VS Code for %s/%s...\n", target.Fleet, target.Instance)
			vscode := exec.Command("code", "--folder-uri", folderURI)
			return vscode.Run()
		},
	}
}
