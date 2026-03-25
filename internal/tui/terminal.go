package tui

import (
	"os"
	"os/exec"
)

// openInTerminal spawns a new terminal window running the given command.
// Tries tmux first, then common Linux terminal emulators.
func openInTerminal(command []string) error {
	// tmux — split or new window
	if os.Getenv("TMUX") != "" {
		args := append([]string{"new-window", "--"}, command...)
		return exec.Command("tmux", args...).Run()
	}

	// Try common Linux terminal emulators in order
	terminals := []struct {
		bin  string
		args func([]string) []string
	}{
		{"gnome-terminal", func(cmd []string) []string {
			return append([]string{"--"}, cmd...)
		}},
		{"konsole", func(cmd []string) []string {
			return append([]string{"-e"}, cmd...)
		}},
		{"xfce4-terminal", func(cmd []string) []string {
			return append([]string{"-e"}, cmd...)
		}},
		{"alacritty", func(cmd []string) []string {
			return append([]string{"-e"}, cmd...)
		}},
		{"kitty", func(cmd []string) []string {
			return cmd
		}},
		{"xterm", func(cmd []string) []string {
			return append([]string{"-e"}, cmd...)
		}},
	}

	for _, t := range terminals {
		if _, err := exec.LookPath(t.bin); err == nil {
			args := t.args(command)
			return exec.Command(t.bin, args...).Start()
		}
	}

	return exec.ErrNotFound
}
