package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	tea "github.com/charmbracelet/bubbletea"
)

// splitPaneMsg is sent after a tmux split-window command completes.
type splitPaneMsg struct {
	paneID   string // tmux pane ID (e.g. "%3")
	instance string // instance name occupying the pane
	session  string // tmux session name in the pane
	groupID  string // session group ID (for group management)
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
func splitPaneCmd(existingPaneID string, instanceName string, sessionName string, groupID string, cmd *exec.Cmd) tea.Cmd {
	// Snapshot the args — we must not capture the *exec.Cmd across goroutines.
	args := cmd.Args

	return func() tea.Msg {
		// Wrap the command so that on non-zero exit the pane stays open
		// briefly, giving the user time to read the error message.
		shellScript := quoteArgs(args) + `; __rc=$?; if [ $__rc -ne 0 ]; then echo; echo "exited with code $__rc — closing in 3s"; sleep 3; fi; exit $__rc`

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
				return splitPaneMsg{paneID: existingPaneID, instance: instanceName, session: sessionName, groupID: groupID}
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
		return splitPaneMsg{paneID: paneID, instance: instanceName, session: sessionName, groupID: groupID}
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

// bindHostSplitKeys rebinds the outer tmux's % and " keys so that
// new splits open a shell inside the given instance (via fleet shell)
// instead of spawning a local shell. When groupID is non-empty, new
// panes are added to the same session group.
func bindHostSplitKeys(instanceName, groupID string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	shellCmd := fmt.Sprintf("%s shell %s", self, instanceName)
	if groupID != "" {
		shellCmd += fmt.Sprintf(" --group %s", groupID)
	}
	_ = exec.Command("tmux", "bind-key", "%", "split-window", "-h", shellCmd).Run()
	_ = exec.Command("tmux", "bind-key", `"`, "split-window", "-v", shellCmd).Run()
}

// unbindHostSplitKeys restores the default tmux split-window bindings.
func unbindHostSplitKeys() {
	_ = exec.Command("tmux", "bind-key", "%", "split-window", "-h").Run()
	_ = exec.Command("tmux", "bind-key", `"`, "split-window", "-v").Run()
}

// unbindDefaultSplitKeys disables the default split-window bindings so
// the user doesn't accidentally create host shell panes before selecting
// an instance. A display-message reminds them to use the TUI.
func unbindDefaultSplitKeys() {
	msg := "Select an instance in fleet first"
	_ = exec.Command("tmux", "bind-key", "%", "display-message", msg).Run()
	_ = exec.Command("tmux", "bind-key", `"`, "display-message", msg).Run()
}

// killAllSplitPanes kills all panes except the TUI pane (index 0).
// Selects the TUI pane first so kill-pane -a removes the right targets
// regardless of which pane currently has focus.
func killAllSplitPanes() {
	_ = exec.Command("tmux", "select-pane", "-t", ":.0").Run()
	_ = exec.Command("tmux", "kill-pane", "-a").Run()
}

// bindHostCloseKeys binds Ctrl+Q and Ctrl+O on the outer tmux root
// table to close all split panes (select TUI pane, then kill others).
func bindHostCloseKeys() {
	// Use run-shell to chain tmux commands reliably. The guard
	// prevents errors when there's only the TUI pane.
	script := `tmux select-pane -t :.0 && tmux kill-pane -a 2>/dev/null || true`
	_ = exec.Command("tmux", "bind-key", "-n", "C-q", "run-shell", script).Run()
	_ = exec.Command("tmux", "bind-key", "-n", "C-o", "run-shell", script).Run()
}

// unbindHostCloseKeys removes the Ctrl+Q and Ctrl+O bindings from the
// outer tmux root table.
func unbindHostCloseKeys() {
	_ = exec.Command("tmux", "unbind", "-n", "C-q").Run()
	_ = exec.Command("tmux", "unbind", "-n", "C-o").Run()
}

// tmuxLayoutString returns the current tmux window layout string.
func tmuxLayoutString() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{window_layout}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// paneSessionOrder reads outer tmux pane titles to determine the
// session-to-pane-position mapping. fleet shell sets each pane's title
// to the inner tmux session name, so we can read them back in pane
// index order. The TUI pane (index 0) is skipped.
func paneSessionOrder() []string {
	out, err := exec.Command("tmux", "list-panes", "-F", "#{pane_index}:#{pane_title}").Output()
	if err != nil {
		return nil
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip the TUI pane (index 0).
		if strings.HasPrefix(line, "0:") {
			continue
		}
		// Extract title after the first colon.
		if idx := strings.IndexByte(line, ':'); idx >= 0 {
			title := line[idx+1:]
			if title != "" {
				sessions = append(sessions, title)
			}
		}
	}
	return sessions
}

// saveCurrentGroupLayout saves the active group's outer tmux layout so
// it can be restored later. Pane titles (set by fleet shell) are read
// in pane index order to preserve the session-to-position mapping.
func (m *model) saveCurrentGroupLayout() {
	if m.activeGroupID == "" || m.splitInstance == "" {
		return
	}

	// Read session names from outer tmux pane titles, in pane order.
	sessionNames := paneSessionOrder()

	// Fallback: if pane titles aren't available, use the one we know.
	if len(sessionNames) == 0 && m.splitSession != "" {
		sessionNames = []string{m.splitSession}
	}

	m.savedGroups[m.activeGroupID] = savedGroup{
		GroupID:      m.activeGroupID,
		InstanceName: m.splitInstance,
		Sessions:     sessionNames,
		Layout:       tmuxLayoutString(),
		PaneCount:    len(sessionNames),
	}
}

// restoreGroupCmd recreates outer tmux panes for a saved session group.
// Instead of trusting the saved session list (which may be stale), it
// queries the inner tmux directly for sessions matching the group prefix.
// Each discovered session gets its own pane via `fleet shell --session`.
func (m *model) restoreGroupCmd(inst *fleet.Instance, groupID string) tea.Cmd {
	b := m.instanceBackend(inst)
	instanceName := inst.Name
	workspaceDir := inst.WorkspaceDir
	sanitized := SanitizeSessionName(instanceName)
	prefix := sanitized + groupSep + groupID

	// Grab saved layout if available.
	savedLayout := ""
	if sg, ok := m.savedGroups[groupID]; ok {
		savedLayout = sg.Layout
	}

	// Prefer saved session order (from pane titles) to preserve
	// the exact pane-to-session mapping.
	var savedOrder []string
	if sg, ok := m.savedGroups[groupID]; ok && len(sg.Sessions) > 0 {
		savedOrder = sg.Sessions
	}

	return func() tea.Msg {
		self, err := os.Executable()
		if err != nil {
			return splitPaneMsg{err: fmt.Errorf("os.Executable: %w", err)}
		}

		// Query the inner tmux for all sessions in this group.
		listCmd := b.ExecCommand(workspaceDir, []string{
			"sh", "-c",
			`tmux list-sessions -F "#{session_name}" 2>/dev/null`,
		})
		out, _ := listCmd.Output()

		// Build a set of live sessions for validation.
		live := make(map[string]bool)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			name := strings.TrimSpace(line)
			if name != "" && strings.HasPrefix(name, prefix) {
				live[name] = true
			}
		}

		// Use saved order if available, filtering to sessions that
		// still exist. Then append any newly discovered sessions
		// that weren't in the saved order.
		var sessions []string
		seen := make(map[string]bool)
		for _, s := range savedOrder {
			if live[s] {
				sessions = append(sessions, s)
				seen[s] = true
			}
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			name := strings.TrimSpace(line)
			if name != "" && strings.HasPrefix(name, prefix) && !seen[name] {
				sessions = append(sessions, name)
			}
		}

		if len(sessions) == 0 {
			return splitPaneMsg{err: fmt.Errorf("no sessions found for group %s", groupID)}
		}

		// Kill any existing split panes.
		_ = exec.Command("tmux", "kill-pane", "-a").Run()

		var firstPaneID string
		for i, sessName := range sessions {
			shellCmd := fmt.Sprintf("%s shell %s --session %s", self, instanceName, sessName)
			script := shellCmd + `; __rc=$?; if [ $__rc -ne 0 ]; then echo; echo "exited with code $__rc — closing in 3s"; sleep 3; fi; exit $__rc`

			var tmuxArgs []string
			if i == 0 {
				// First pane: horizontal split from TUI.
				tmuxArgs = []string{
					"split-window", "-h",
					"-l", "70%",
					"-P", "-F", "#{pane_id}",
					"--", "sh", "-c", script,
				}
			} else {
				// Subsequent panes: vertical split in the right area.
				tmuxArgs = []string{
					"split-window", "-v",
					"-t", firstPaneID,
					"-P", "-F", "#{pane_id}",
					"--", "sh", "-c", script,
				}
			}

			paneOut, err := exec.Command("tmux", tmuxArgs...).Output()
			if err != nil {
				continue
			}
			paneID := strings.TrimSpace(string(paneOut))
			if i == 0 {
				firstPaneID = paneID
			}
		}

		// Try to restore the saved layout.
		if savedLayout != "" && firstPaneID != "" {
			_ = exec.Command("tmux", "select-layout", savedLayout).Run()
		}

		if firstPaneID != "" {
			_ = exec.Command("tmux", "select-pane", "-t", firstPaneID).Run()
		}

		// Wait briefly for panes to initialize, then force a repaint
		// to avoid blank/corrupted terminals after rapid pane creation.
		time.Sleep(2 * time.Second)
		_ = exec.Command("tmux", "refresh-client").Run()

		firstSession := ""
		if len(sessions) > 0 {
			firstSession = sessions[0]
		}
		return splitPaneMsg{
			paneID:   firstPaneID,
			instance: instanceName,
			session:  firstSession,
			groupID:  groupID,
		}
	}
}

// unbindHostSessionKeys removes C-PPage and C-NPage from the host
// tmux root table so they pass through to inner tmux sessions.
func unbindHostSessionKeys() {
	_ = exec.Command("tmux", "unbind", "-T", "root", "C-PPage").Run()
	_ = exec.Command("tmux", "unbind", "-T", "root", "C-NPage").Run()
}

// rebindHostSessionKeys restores the default C-PPage/C-NPage bindings
// on the host tmux (copy-mode related defaults).
func rebindHostSessionKeys() {
	_ = exec.Command("tmux", "bind", "-T", "root", "C-PPage", "copy-mode", "-eu").Run()
	_ = exec.Command("tmux", "bind", "-T", "root", "C-NPage", "send-keys", "PPage").Run()
}
