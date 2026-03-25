package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [fleet]",
		Aliases: []string{"ls"},
		Short:   "List devcontainer instances",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := state.Load()
			if err != nil {
				return err
			}

			var fleetFilter string
			if len(args) == 1 {
				fleetFilter = args[0]
			} else {
				// Try to infer from cwd
				fleetFilter, _ = fleet.FleetNameFromCwd()
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "FLEET\tINSTANCE\tSTATUS\tCONTAINER\tCREATED")

			for name, f := range st.Fleets {
				if fleetFilter != "" && name != fleetFilter {
					continue
				}
				for _, inst := range f.Instances {
					containerShort := inst.ContainerID
					if len(containerShort) > 12 {
						containerShort = containerShort[:12]
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
						name,
						inst.Name,
						inst.Status,
						containerShort,
						inst.CreatedAt.Format("2006-01-02 15:04"),
					)
				}
			}

			w.Flush()
			return nil
		},
	}

	return cmd
}
