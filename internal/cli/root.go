package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/BenjaminBenetti/fleet-man/internal/doctor"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/BenjaminBenetti/fleet-man/internal/tui"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root fleet command.
func NewRootCmd() *cobra.Command {
	var useTmux bool
	var runDoctor bool

	root := &cobra.Command{
		Use:   "fleet",
		Short: "Manage fleets of devcontainers",
		Long:  "fleet-man is a CLI/TUI tool for spawning and managing fleets of devcontainers from a repo.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runDoctor {
				return doctor.Run()
			}
			if useTmux {
				return relaunchInTmux()
			}
			return tui.Run()
		},
	}

	root.Flags().BoolVar(&useTmux, "tmux", false, "launch fleet inside a tmux session (enables split pane mode)")
	root.Flags().BoolVar(&runDoctor, "doctor", false, "launch a coding agent to diagnose and fix your setup")

	root.AddCommand(
		newUpCmd(),
		newStopCmd(),
		newStartCmd(),
		newDownCmd(),
		newDestroyCmd(),
		newListCmd(),
		newExecCmd(),
		newCodeCmd(),
		newLogsCmd(),
		newStatusCmd(),
		newCreateInstanceCmd(),
		newPortForwardCmd(),
	)

	return root
}

// relaunchInTmux re-execs the fleet binary inside a new tmux session.
// If already inside tmux, it runs the TUI directly.
func relaunchInTmux() error {
	if os.Getenv("TMUX") != "" {
		// Already in tmux — just run the TUI.
		return tui.Run()
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("os.Executable: %w", err)
	}

	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// exec into: tmux new-session -s fleet -- <self>
	// This replaces the current process so the user gets a clean
	// tmux session running fleet with split pane mode enabled.
	// Generate a unique session name so multiple fleet instances can coexist.
	var suffix [3]byte
	_, _ = rand.Read(suffix[:])
	session := "fleet-" + hex.EncodeToString(suffix[:])

	// exec into tmux: create a session running fleet, then enable mouse.
	// tmux processes `;`-separated commands as part of startup.
	// Vim-style pane navigation (h/l) is injected only when the user
	// has enabled the "tmux vim keys" setting.
	args := []string{
		"tmux", "new-session", "-s", session, self,
		";", "set", "-g", "mouse", "on",
	}

	cfg, _ := state.LoadConfig()
	if cfg == nil || cfg.GeneralSettings.TmuxVimKeysEnabled() {
		args = append(args,
			";", "bind-key", "h", "select-pane", "-L",
			";", "bind-key", "l", "select-pane", "-R",
		)
	}

	return syscall.Exec(tmuxBin, args, os.Environ())
}
