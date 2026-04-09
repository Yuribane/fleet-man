package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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

// killAllSplitPanes kills all panes except the current one (the TUI pane).
// This is used when switching session groups.
func killAllSplitPanes() {
	_ = exec.Command("tmux", "kill-pane", "-a").Run()
}

// tmuxLayoutString returns the current tmux window layout string.
func tmuxLayoutString() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{window_layout}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// saveCurrentGroupLayout saves the active group's outer tmux layout so
// it can be restored later.
func (m *model) saveCurrentGroupLayout() {
	if m.activeGroupID == "" || m.splitInstance == "" {
		return
	}
	// Discover which sessions belong to this group by querying the
	// inner tmux. We do this synchronously since we're about to kill
	// all panes and need the info now.
	sanitized := SanitizeSessionName(m.splitInstance)
	instKey := ""
	// Find the instance key for session discovery.
	for key, disc := range m.sessions {
		if disc == nil || disc.err != nil {
			continue
		}
		for _, s := range disc.sessions {
			gid, ok := parseGroupID(sanitized, s.Name)
			if ok && gid == m.activeGroupID {
				instKey = key
				break
			}
		}
		if instKey != "" {
			break
		}
	}

	var sessionNames []string
	if instKey != "" {
		if disc, ok := m.sessions[instKey]; ok && disc.err == nil {
			for _, s := range disc.sessions {
				gid, ok := parseGroupID(sanitized, s.Name)
				if ok && gid == m.activeGroupID {
					sessionNames = append(sessionNames, s.Name)
				}
			}
		}
	}
	// If discovery didn't find sessions yet, at least save the one we know.
	if len(sessionNames) == 0 && m.splitSession != "" {
		sessionNames = []string{m.splitSession}
	}

	// Count outer tmux panes (excluding TUI pane).
	paneCount := 0
	if out, err := exec.Command("tmux", "list-panes", "-F", "x").Output(); err == nil {
		paneCount = strings.Count(strings.TrimSpace(string(out)), "x") - 1 // subtract TUI pane
	}

	m.savedGroups[m.activeGroupID] = savedGroup{
		GroupID:      m.activeGroupID,
		InstanceName: m.splitInstance,
		Sessions:     sessionNames,
		Layout:       tmuxLayoutString(),
		PaneCount:    paneCount,
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

		var sessions []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			name := strings.TrimSpace(line)
			if name != "" && strings.HasPrefix(name, prefix) {
				sessions = append(sessions, name)
			}
		}

		if len(sessions) == 0 {
			// Group has no surviving sessions — fall back to creating
			// a fresh one in the group.
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
