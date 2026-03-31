package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/create"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	var repoFlag string
	var backendFlag string

	cmd := &cobra.Command{
		Use:   "up <name>",
		Short: "Spawn a new instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			bt := fleet.BackendDevcontainer
			if backendFlag == "coder" {
				bt = fleet.BackendCoder
			}

			target, err := fleet.Resolve(name, repoFlag)
			if err != nil {
				return err
			}

			st, err := state.Load()
			if err != nil {
				return err
			}

			// Determine the remote URL
			remoteURL := repoFlag
			if remoteURL == "" {
				if f, ok := st.Fleets[target.Fleet]; ok {
					remoteURL = f.Remote
				} else {
					remoteURL, err = fleet.RemoteURLFromCwd()
					if err != nil {
						return fmt.Errorf("could not determine repo URL: %w", err)
					}
				}
			}

			f := st.GetOrCreateFleet(target.Fleet, remoteURL)

			// Check if instance already exists
			if _, err := f.GetInstance(target.Instance); err == nil {
				return fmt.Errorf("instance %s/%s already exists", target.Fleet, target.Instance)
			}

			// Pre-create instance in state with "creating" status
			wsDir := filepath.Join(state.WorkspacesDir(), target.Fleet, target.Instance, target.Fleet)
			inst := &fleet.Instance{
				Name:         target.Instance,
				Config:       ".devcontainer/devcontainer.json",
				WorkspaceDir: wsDir,
				CreatedAt:    time.Now(),
				Status:       fleet.StatusCreating,
				Backend:      bt,
			}
			if err := f.AddInstance(inst); err != nil {
				return err
			}
			if err := state.Save(st); err != nil {
				return err
			}

			fmt.Printf("Creating %s/%s (backend: %s)...\n", target.Fleet, target.Instance, bt)
			if err := create.Run(target.Fleet, target.Instance, remoteURL, true, bt); err != nil {
				return err
			}

			// Reload state to get the updated container ID
			st, err = state.Load()
			if err != nil {
				return err
			}
			if f, ok := st.Fleets[target.Fleet]; ok {
				if inst, err := f.GetInstance(target.Instance); err == nil {
					fmt.Printf("Instance %s/%s is running (container: %s)\n", target.Fleet, target.Instance, inst.ContainerID[:min(12, len(inst.ContainerID))])
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoFlag, "repo", "", "Git remote URL to clone from")
	cmd.Flags().StringVar(&backendFlag, "backend", "devcontainer", "Backend type: devcontainer or coder")
	return cmd
}
