package cli

import (
	"fmt"

	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
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
			stopped := 0
			other := 0

			for name, f := range st.Fleets {
				fleetRunning := 0
				fleetStopped := 0
				fleetOther := 0
				for _, inst := range f.Instances {
					totalInstances++
					switch inst.Status {
					case fleet.StatusRunning:
						fleetRunning++
						running++
					case fleet.StatusStopped:
						fleetStopped++
						stopped++
					default:
						fleetOther++
						other++
					}
				}
				fmt.Printf("%s: %d instances (%s) — %s\n", name, len(f.Instances), formatStatusCounts(fleetRunning, fleetStopped, fleetOther), f.Remote)
			}

			fmt.Printf("\nTotal: %d fleets, %d instances (%s)\n", len(st.Fleets), totalInstances, formatStatusCounts(running, stopped, other))
			return nil
		},
	}
}

func formatStatusCounts(running, stopped, other int) string {
	if other > 0 {
		return fmt.Sprintf("%d running, %d stopped, %d other", running, stopped, other)
	}
	return fmt.Sprintf("%d running, %d stopped", running, stopped)
}
