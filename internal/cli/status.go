package cli

import (
	"fmt"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show fleet-wide status summary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := state.Load()
			if err != nil {
				return err
			}

			if len(st.Fleets) == 0 {
				fmt.Println("No fleets. Use 'fleet up <name>' to create an instance.")
				return nil
			}

			totalInstances := 0
			running := 0

			for name, f := range st.Fleets {
				fleetRunning := 0
				for _, inst := range f.Instances {
					totalInstances++
					if inst.Status == "running" {
						fleetRunning++
						running++
					}
				}
				fmt.Printf("%s: %d instances (%d running) — %s\n", name, len(f.Instances), fleetRunning, f.Remote)
			}

			fmt.Printf("\nTotal: %d fleets, %d instances (%d running)\n", len(st.Fleets), totalInstances, running)
			return nil
		},
	}
}
