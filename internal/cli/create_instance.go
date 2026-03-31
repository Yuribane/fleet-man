package cli

import (
	"github.com/BenjaminBenetti/fleet-man/internal/create"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/spf13/cobra"
)

func newCreateInstanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_create-instance",
		Short:  "Internal: create an instance in the background",
		Hidden: true,
		Args:   cobra.RangeArgs(3, 4), // fleetName instanceName remoteURL [backendType]
		RunE: func(cmd *cobra.Command, args []string) error {
			bt := fleet.BackendDevcontainer
			if len(args) >= 4 {
				bt = fleet.BackendType(args[3])
			}
			return create.Run(args[0], args[1], args[2], false, bt)
		},
	}
}
