package cli

import (
	"fmt"

	"github.com/BenjaminBenetti/fleet-man/internal/backendutil"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec <name> <command...>",
		Short: "Execute a command inside an instance",
		Args:  cobra.MinimumNArgs(2),
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

			dc := backendutil.NewForInstance(inst, false)
			return dc.Exec(inst.WorkspaceDir, args[1:])
		},
	}
}
