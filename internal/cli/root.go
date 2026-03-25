package cli

import (
	"github.com/fleet-man/fleet-man/internal/tui"
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "Manage fleets of devcontainers",
		Long:  "fleet-man is a CLI/TUI tool for spawning and managing fleets of devcontainers from a repo.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run()
		},
	}

	root.AddCommand(
		newUpCmd(),
		newDownCmd(),
		newListCmd(),
		newExecCmd(),
		newCodeCmd(),
		newLogsCmd(),
		newStatusCmd(),
	)

	return root
}
