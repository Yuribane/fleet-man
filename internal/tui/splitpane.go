package tui

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// splitPaneMsg is sent after a tmux split-window command completes.
type splitPaneMsg struct {
	paneID   string // tmux pane ID (e.g. "%3")
	instance string // instance name occupying the pane
	err      error
}

// tmuxWindowSize queries the host tmux for the current window dimensions.
// Returns (cols, rows) or (0, 0) if the query fails.
func tmuxWindowSize() (int, int) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{window_width} #{window_height}").Output()
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}

// quoteArgs builds a shell-safe command string from exec args.
func quoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}

// splitPaneCmd creates or replaces the right-side tmux pane with the given
// command. When an existing pane ID is provided, it is respawned in-place
// (via respawn-pane) to avoid layout changes that cause visual corruption.
// If the pane no longer exists, it falls back to creating a fresh split.
func splitPaneCmd(existingPaneID string, instanceName string, cmd *exec.Cmd) tea.Cmd {
	// Snapshot the args — we must not capture the *exec.Cmd across goroutines.
	args := cmd.Args

	return func() tea.Msg {
		shellScript := quoteArgs(args)

		// If we have an existing pane, respawn it in-place to avoid
		// layout changes that cause visual corruption in the fleet TUI.
		if existingPaneID != "" {
			respawnArgs := []string{
				"respawn-pane", "-k",
				"-t", existingPaneID,
				"sh", "-c", shellScript,
			}
			if exec.Command("tmux", respawnArgs...).Run() == nil {
				_ = exec.Command("tmux", "select-pane", "-t", existingPaneID).Run()
				return splitPaneMsg{paneID: existingPaneID, instance: instanceName}
			}
			// Pane is gone — fall through to create a fresh split.
		}

		// Kill any stale sibling panes before creating a fresh split.
		_ = exec.Command("tmux", "kill-pane", "-a").Run()

		// Create a horizontal split (side by side). -P -F prints the new
		// pane ID so we can track it. -l 70% gives the shell pane 70% width.
		tmuxArgs := []string{
			"split-window", "-h",
			"-l", "70%",
			"-P", "-F", "#{pane_id}",
			"--", "sh", "-c", shellScript,
		}
		out, err := exec.Command("tmux", tmuxArgs...).Output()
		if err != nil {
			return splitPaneMsg{err: fmt.Errorf("split-window: %w", err)}
		}

		paneID := strings.TrimSpace(string(out))
		_ = exec.Command("tmux", "select-pane", "-t", paneID).Run()
		return splitPaneMsg{paneID: paneID, instance: instanceName}
	}
}

// splitOpen returns true if the current tmux window has more than one
// pane, meaning the split is still visible. More reliable than checking
// a specific pane ID, which can go stale.
func splitOpen() bool {
	out, err := exec.Command("tmux", "list-panes", "-F", "x").Output()
	if err != nil {
		return false
	}
	return strings.Count(strings.TrimSpace(string(out)), "x") > 1
}

// killSplitPane kills the tracked tmux pane if it exists. Safe to call
// with an empty paneID (no-op).
func killSplitPane(paneID string) {
	if paneID == "" {
		return
	}
	_ = exec.Command("tmux", "kill-pane", "-t", paneID).Run()
}
