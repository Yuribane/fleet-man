package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fleet-man/fleet-man/internal/devcontainer"
	"github.com/fleet-man/fleet-man/internal/fleet"
	"github.com/fleet-man/fleet-man/internal/state"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	var repoFlag string

	cmd := &cobra.Command{
		Use:   "up <name>",
		Short: "Spawn a new devcontainer instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

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

			// Clone the repo into the workspace directory
			wsDir := filepath.Join(state.WorkspacesDir(), target.Fleet, target.Instance, target.Fleet)
			fmt.Printf("Cloning %s into %s...\n", remoteURL, wsDir)
			if err := os.MkdirAll(filepath.Dir(wsDir), 0755); err != nil {
				return fmt.Errorf("creating workspace parent dir: %w", err)
			}

			gitClone := exec.Command("git", "clone", remoteURL, wsDir)
			gitClone.Stdout = os.Stdout
			gitClone.Stderr = os.Stderr
			if err := gitClone.Run(); err != nil {
				return fmt.Errorf("git clone failed: %w", err)
			}

			// Run devcontainer up
			fmt.Printf("Starting devcontainer for %s/%s...\n", target.Fleet, target.Instance)
			dc := devcontainer.NewClient()
			dc.Verbose = true
			result, err := dc.Up(wsDir)
			if err != nil {
				return fmt.Errorf("devcontainer up failed: %w", err)
			}

			// Record the instance
			inst := &fleet.Instance{
				Name:         target.Instance,
				ContainerID:  result.ContainerID,
				Config:       ".devcontainer/devcontainer.json",
				WorkspaceDir: wsDir,
				CreatedAt:    time.Now(),
				Status:       fleet.StatusRunning,
			}

			if err := f.AddInstance(inst); err != nil {
				return err
			}

			if err := state.Save(st); err != nil {
				return err
			}

			fmt.Printf("Instance %s/%s is running (container: %s)\n", target.Fleet, target.Instance, result.ContainerID[:12])
			return nil
		},
	}

	cmd.Flags().StringVar(&repoFlag, "repo", "", "Git remote URL to clone from")
	return cmd
}
