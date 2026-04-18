package cli

import (
	"github.com/BenjaminBenetti/fleet-man/internal/create"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/spf13/cobra"
)

// newCreateInstanceCmd returns the hidden `_create-instance` subcommand
// the TUI spawns as a detached child to perform the actual instance
// provisioning. It accepts the fleet, instance, and repo as positional
// args; backend type and branch are supplied via flags so callers can
// omit either without positional ambiguity.
func newCreateInstanceCmd() *cobra.Command {
	var backendFlag string
	var branchFlag string

	cmd := &cobra.Command{
		Use:    "_create-instance",
		Short:  "Internal: create an instance in the background",
		Hidden: true,
		Args:   cobra.RangeArgs(3, 4), // fleetName instanceName remoteURL [backendType]
		RunE: func(cmd *cobra.Command, args []string) error {
			bt := fleet.BackendDevcontainer
			if backendFlag != "" {
				bt = fleet.BackendType(backendFlag)
			} else if len(args) >= 4 {
				// Preserve the legacy 4th positional form for compatibility
				// with older callers that predate the --backend flag.
				bt = fleet.BackendType(args[3])
			}
			return create.Run(args[0], args[1], args[2], branchFlag, false, bt)
		},
	}

	cmd.Flags().StringVar(&backendFlag, "backend", "", "Backend type: devcontainer, coder, or codespaces")
	cmd.Flags().StringVar(&branchFlag, "branch", "", "Git branch to check out")
	return cmd
}
