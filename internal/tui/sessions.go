package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	tea "github.com/charmbracelet/bubbletea"
)

// ===========================================
// Types
// ===========================================

// tmuxSession represents a discovered tmux session inside a container.
type tmuxSession struct {
	Name     string
	Windows  int
	Attached bool
}

// sessionDiscovery holds discovered sessions for a single instance.
type sessionDiscovery struct {
	sessions  []tmuxSession
	err       error
	fetchedAt time.Time
}

// sessionPoller manages the fast 1-second session polling loop with
// a lock to prevent concurrent polls from stacking up.
type sessionPoller struct {
	running atomic.Bool // true while a poll is in flight
}

// newSessionPoller creates a new session poller.
func newSessionPoller() *sessionPoller {
	return &sessionPoller{}
}

// ===========================================
// Messages
// ===========================================

// sessionsMsg is sent after listing tmux sessions inside a container.
type sessionsMsg struct {
	instanceKey string // "fleet/instance"
	sessions    []tmuxSession
	err         error
}

// sessionPollMsg carries the combined result of a fast session poll:
// discovered sessions per expanded instance and the active session
// per container.
type sessionPollMsg struct {
	discovered     map[string][]tmuxSession // instanceKey → sessions
	activeSessions map[string]string        // containerID → active session name
}

// sessionCreatedMsg is sent after creating a new tmux session.
type sessionCreatedMsg struct {
	instanceKey string
	err         error
}

// sessionRenamedMsg is sent after renaming a tmux session.
type sessionRenamedMsg struct {
	instanceKey string
	oldName     string
	newName     string
	err         error
}

// ===========================================
// Session Polling
// ===========================================

// sessionPollCmd runs a fast session poll: lists sessions for expanded
// instances and detects active sessions for all running containers.
// A lock prevents concurrent polls from stacking. If a previous poll
// is still in flight, this one is skipped. A 30-second timeout acts
// as a safety valve for slow remote hosts.
func sessionPollCmd(
	poller *sessionPoller,
	backends map[fleet.BackendType]backend.Backend,
	expanded map[string]bool,
	fleets map[string]*fleet.Fleet,
	delay bool,
) tea.Cmd {
	// Snapshot what we need — don't capture model fields across goroutines.
	type target struct {
		instanceKey  string
		workspaceDir string
		containerID  string
		backendType  fleet.BackendType
		isExpanded   bool
	}
	var targets []target
	for _, f := range fleets {
		for _, inst := range f.Instances {
			if inst.Status != fleet.StatusRunning || inst.ContainerID == "" {
				continue
			}
			bt := inst.Backend
			if bt == "" {
				bt = fleet.BackendDevcontainer
			}
			instKey := f.Name + "/" + inst.Name
			targets = append(targets, target{
				instanceKey:  instKey,
				workspaceDir: inst.WorkspaceDir,
				containerID:  inst.ContainerID,
				backendType:  bt,
				isExpanded:   expanded[instKey],
			})
		}
	}

	return func() tea.Msg {
		if delay {
			time.Sleep(1 * time.Second)
		}

		// Skip if a previous poll is still running.
		if !poller.running.CompareAndSwap(false, true) {
			return sessionPollMsg{}
		}
		defer poller.running.Store(false)

		if len(targets) == 0 {
			return sessionPollMsg{}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		discovered := make(map[string][]tmuxSession)
		activeSessions := make(map[string]string)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, t := range targets {
			b := backends[t.backendType]
			if b == nil {
				continue
			}

			// Always poll active session.
			wg.Add(1)
			go func(b backend.Backend, cid string) {
				defer wg.Done()
				sess := b.ActiveSession(cid)
				if sess != "" {
					mu.Lock()
					activeSessions[cid] = sess
					mu.Unlock()
				}
			}(b, t.containerID)

			// Only list sessions for expanded instances.
			if t.isExpanded {
				wg.Add(1)
				go func(b backend.Backend, wsDir, instKey string) {
					defer wg.Done()
					cmd := b.ExecCommand(wsDir, []string{
						"sh", "-c",
						`tmux list-sessions -F "#{session_name}:#{session_windows}:#{session_attached}" 2>/dev/null`,
					})
					out, err := cmd.Output()
					if err != nil {
						return
					}
					sessions := parseTmuxSessions(string(out))
					mu.Lock()
					discovered[instKey] = sessions
					mu.Unlock()
				}(b, t.workspaceDir, t.instanceKey)
			}
		}

		// Wait for all goroutines or timeout.
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
		}

		return sessionPollMsg{discovered: discovered, activeSessions: activeSessions}
	}
}

// listSessionsCmd returns a tea.Cmd that execs `tmux list-sessions`
// inside the container and parses the output into tmuxSession structs.
func listSessionsCmd(b backend.Backend, workspaceDir, instanceKey string) tea.Cmd {
	return func() tea.Msg {
		cmd := b.ExecCommand(workspaceDir, []string{
			"sh", "-c",
			`tmux list-sessions -F "#{session_name}:#{session_windows}:#{session_attached}" 2>/dev/null`,
		})
		out, err := cmd.Output()
		if err != nil {
			return sessionsMsg{instanceKey: instanceKey, err: err}
		}
		sessions := parseTmuxSessions(string(out))
		return sessionsMsg{instanceKey: instanceKey, sessions: sessions}
	}
}

// createSessionCmd execs `tmux new-session -d -s <name>` inside the
// container and then re-lists sessions to refresh the UI.
func createSessionCmd(b backend.Backend, workspaceDir, instanceKey, sessionName string) tea.Cmd {
	return func() tea.Msg {
		cmd := b.ExecCommand(workspaceDir, []string{
			"sh", "-c",
			fmt.Sprintf(`tmux new-session -d -s %s 2>/dev/null`, shQuote(sessionName)),
		})
		if err := cmd.Run(); err != nil {
			return sessionCreatedMsg{instanceKey: instanceKey, err: err}
		}
		return sessionCreatedMsg{instanceKey: instanceKey}
	}
}

// renameSessionCmd execs `tmux rename-session -t <old> <new>` inside
// the container.
func renameSessionCmd(b backend.Backend, workspaceDir, instanceKey, oldName, newName string) tea.Cmd {
	return func() tea.Msg {
		cmd := b.ExecCommand(workspaceDir, []string{
			"sh", "-c",
			fmt.Sprintf(`tmux rename-session -t %s %s 2>/dev/null`, shQuote(oldName), shQuote(newName)),
		})
		if err := cmd.Run(); err != nil {
			return sessionRenamedMsg{instanceKey: instanceKey, oldName: oldName, newName: newName, err: err}
		}
		return sessionRenamedMsg{instanceKey: instanceKey, oldName: oldName, newName: newName}
	}
}

// ===========================================
// Helpers
// ===========================================

// parseTmuxSessions parses the output of `tmux list-sessions -F
// "#{session_name}:#{session_windows}:#{session_attached}"` into
// a slice of tmuxSession.
func parseTmuxSessions(output string) []tmuxSession {
	var sessions []tmuxSession
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}
		s := tmuxSession{Name: parts[0]}
		if len(parts) >= 2 {
			s.Windows, _ = strconv.Atoi(parts[1])
		}
		if len(parts) >= 3 {
			s.Attached = parts[2] == "1"
		}
		sessions = append(sessions, s)
	}
	return sessions
}

// nextSessionName generates an auto-incrementing session name like
// "session-2", "session-3", etc. based on existing sessions.
func nextSessionName(existing []tmuxSession) string {
	maxN := 1
	for _, s := range existing {
		if strings.HasPrefix(s.Name, "session-") {
			if n, err := strconv.Atoi(strings.TrimPrefix(s.Name, "session-")); err == nil && n > maxN {
				maxN = n
			}
		}
	}
	return fmt.Sprintf("session-%d", maxN+1)
}
