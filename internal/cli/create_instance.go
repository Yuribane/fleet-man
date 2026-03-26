package cli

import (
	"github.com/BenjaminBenetti/fleet-man/internal/create"
	"github.com/spf13/cobra"
)

func newCreateInstanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_create-instance",
		Short:  "Internal: create an instance in the background",
		Hidden: true,
		Args:   cobra.ExactArgs(3), // fleetName instanceName remoteURL
		RunE: func(cmd *cobra.Command, args []string) error {
			return create.Run(args[0], args[1], args[2], false)
		},
	}
}
