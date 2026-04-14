package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/backendutil"
	"github.com/BenjaminBenetti/fleet-man/internal/deps"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/gitutil"
	"github.com/BenjaminBenetti/fleet-man/internal/instanceops"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/BenjaminBenetti/fleet-man/internal/version"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var toggleInstanceStatus = instanceops.ToggleInstance
var resolveWorkspaceBranch = gitutil.BranchName

func (m model) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.message = ""

		switch msg.String() {
		case "q", "ctrl+c", "ctrl+q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			m.moveCursor(-1)

		case "down", "j":
			m.moveCursor(1)

		case " ", "tab":
			// Toggle collapse on fleet headers, expand/collapse instances,
			// or act like enter on session/new-session/settings rows.
			if r := m.currentRow(); r != nil {
				switch r.kind {
				case rowFleetHeader:
					name := r.fleetName
					m.collapsed[name] = !m.collapsed[name]
					m.buildRows()
				case rowInstance:
					if r.instance == nil {
						break
					}
					if r.instance.Status != fleet.StatusRunning {
						m.message = "Instance must be running to view sessions"
						break
					}
					instKey := r.fleetName + "/" + r.instance.Name
					if m.expandedInstances[instKey] {
						delete(m.expandedInstances, instKey)
						m.buildRows()
					} else {
						m.expandedInstances[instKey] = true
						m.buildRows()
						b := m.instanceBackend(r.instance)
						return m, listSessionsCmd(b, r.instance.WorkspaceDir, instKey)
					}
				case rowSession, rowNewSession, rowSettings:
					// Delegate to the enter/e handler for these row types.
					return m.handleEnter()
				}
			}

		case "r":
			if r := m.currentRow(); r != nil && r.kind == rowSession {
				m.mode = viewRenameSession
				m.dialogFleet = r.fleetName
				m.dialogInst = r.instance.Name
				m.dialogSession = r.sessionName
				// Show just the group ID (display name) for editing,
				// not the full internal session name.
				displayName := r.sessionName
				if r.instance != nil {
					sanitized := SanitizeSessionName(r.instance.Name)
					if gid, ok := parseGroupID(sanitized, r.sessionName); ok {
						displayName = gid
					}
				}
				m.textInput.SetValue(displayName)
				m.textInput.Placeholder = "new-session-name"
				m.textInput.CharLimit = 64
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			m.reload()
			m.message = "Refreshed"

		case "s":
			r := m.currentRow()
			if r == nil || r.kind != rowInstance || r.instance == nil {
				m.message = "Select an instance"
				break
			}

			key := r.fleetName + "/" + r.instance.Name
			if isTransitional(r.instance.Status) {
				m.message = fmt.Sprintf("Instance %s is %s", key, r.instance.Status)
				break
			}
			if r.instance.Status == fleet.StatusFailed {
				m.message = fmt.Sprintf("Instance %s is failed and cannot be toggled", key)
				break
			}

			// Set transitional status and save immediately
			if r.instance.Status == fleet.StatusRunning {
				r.instance.Status = fleet.StatusStopping
			} else if r.instance.Status == fleet.StatusStopped {
				r.instance.Status = fleet.StatusStarting
			}
			_ = state.Save(m.st)
			m.buildRows()

			fleetName, instName := r.fleetName, r.instance.Name
			return m, toggleInstanceCmd(fleetName, instName)

		case "d":
			r := m.currentRow()
			if r == nil || r.kind == rowSettings || r.kind == rowNewSession {
				break
			}
			if r.kind == rowSession {
				// Session-level delete
				m.dialogFleet = r.fleetName
				m.dialogInst = r.instance.Name
				m.dialogSession = r.sessionName
				m.dialogGroupID = r.groupID
				m.mode = viewConfirmDeleteSession
				break
			}
			m.dialogFleet = r.fleetName
			if r.kind == rowFleetHeader {
				m.dialogInst = "" // empty means fleet-level delete
			} else if r.instance != nil {
				m.dialogInst = r.instance.Name
			} else {
				break
			}
			m.mode = viewConfirmDelete

		case "a":
			r := m.currentRow()
			if r == nil {
				m.message = "No fleet selected"
				break
			}
			// On instance or session rows, create a new session instead
			if r.kind == rowInstance || r.kind == rowSession || r.kind == rowNewSession {
				inst := r.instance
				if inst == nil {
					break
				}
				if inst.Status != fleet.StatusRunning {
					m.message = "Instance must be running to create sessions"
					break
				}
				m.mode = viewCreateSession
				m.dialogFleet = r.fleetName
				m.dialogInst = inst.Name
				m.textInput.SetValue("")
				m.textInput.Placeholder = "session-name (or empty for auto)"
				m.textInput.CharLimit = 64
				m.textInput.Focus()
				return m, m.textInput.Cursor.BlinkCmd()
			}
			// Fleet header or settings row — add instance
			fleetName := m.currentFleetName()
			if fleetName == "" {
				m.message = "No fleet selected"
				break
			}
			m.toolStatus = deps.CheckTools()
			available := m.availableBackendTypes()
			if len(available) == 0 {
				m.message = "No deploy targets available – install devcontainer or coder CLI"
				break
			}
			m.mode = viewAddInstance
			m.dialogFleet = fleetName
			m.dialogBackend = available[0]
			if m.cfg != nil {
				preferred := fleet.BackendType(m.cfg.DefaultBackend)
				for _, bt := range available {
					if bt == preferred {
						m.dialogBackend = preferred
						break
					}
				}
			}
			m.textInput.SetValue("")
			m.textInput.Placeholder = "instance-name"
			m.textInput.CharLimit = 64
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()

		case "n":
			m.mode = viewAddFleet
			m.textInput.SetValue("")
			m.textInput.Placeholder = "git@github.com:org/repo.git"
			m.textInput.CharLimit = 256
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()

		case "pgup", "pgdown":
			// Cycle session groups: Ctrl+PageUp/Down on the outer tmux
			// sends PageUp/PageDown to the TUI pane.
			if m.inHostTmux && m.splitInstance != "" && m.activeGroupID != "" {
				return m.cycleSessionGroup(msg.String() == "pgup")
			}

		case "enter", "e":
			return m.handleEnter()

		case "o":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			cmd := m.instanceBackend(inst).ExecCommand(inst.WorkspaceDir, freshShellCommand(m.cfg))
			err := openInTerminal(cmd.Args)
			if err != nil {
				m.message = fmt.Sprintf("Could not open terminal: %v", err)
			} else {
				m.message = fmt.Sprintf("Opened terminal for %s", inst.Name)
			}

		case "c":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			r := m.rows[m.cursor]
			var codeCmd *exec.Cmd
			switch inst.Backend {
			case fleet.BackendCoder:
				codeCmd = exec.Command("coder", backendutil.CoderOpenVSCodeArgs(inst.ContainerID)...)
			case fleet.BackendCodespaces:
				codeCmd = exec.Command("gh", "codespace", "code", "-c", inst.ContainerID)
			default:
				uri, ok := m.instanceBackend(inst).EditorURI(inst.WorkspaceDir, r.fleetName)
				if !ok {
					m.message = "Editor integration not supported by this backend"
					break
				}
				codeCmd = exec.Command("code", "--folder-uri", uri)
			}
			if err := codeCmd.Run(); err != nil {
				m.message = fmt.Sprintf("VS Code error: %v", err)
			} else {
				m.message = fmt.Sprintf("Opened VS Code for %s", inst.Name)
			}

		case "t":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			m.mode = viewTagInstance
			m.dialogFleet = m.currentFleetName()
			m.dialogInst = inst.Name
			m.textInput.SetValue(inst.Tag)
			m.textInput.Placeholder = "short description"
			m.textInput.CharLimit = 128
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()

		case "l":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			r := m.rows[m.cursor]
			return m, tea.ExecProcess(
				logsCommand(m.instanceBackend(inst), r.fleetName, inst),
				func(err error) tea.Msg { return execDoneMsg{err} },
			)

		case "f":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			if inst.Status != fleet.StatusRunning {
				m.message = fmt.Sprintf("Instance must be running to port-forward (status: %s)", inst.Status)
				break
			}
			m.mode = viewPortForward
			m.dialogFleet = m.currentFleetName()
			m.dialogInst = inst.Name
			m.pfContainerID = inst.ContainerID
			m.pfCursor = 0
			m.textInput.SetValue("")
			m.textInput.Placeholder = "local:remote (e.g. 8080:80)"
			m.textInput.CharLimit = 11
			m.textInput.Focus()
			return m, m.textInput.Cursor.BlinkCmd()
		}

	case execDoneMsg:
		m.reload()
		if msg.err != nil {
			m.message = fmt.Sprintf("Command error: %v", msg.err)
		}
	}

	return m, nil
}

// handleEnter executes the enter/e/space action for the current row.
// Extracted so that both "enter"/"e" and "space" (on session/new-session/
// settings rows) share the same logic.
func (m model) handleEnter() (tea.Model, tea.Cmd) {
	r := m.currentRow()
	if r == nil {
		return m, nil
	}

	switch r.kind {
	case rowSettings:
		m.page = pageSettings
		m.toolStatus = deps.CheckTools()
		return m, nil

	case rowFleetHeader:
		name := r.fleetName
		m.collapsed[name] = !m.collapsed[name]
		m.buildRows()

	case rowNewSession:
		// Open dialog to create a new session
		inst := r.instance
		m.mode = viewCreateSession
		m.dialogFleet = r.fleetName
		m.dialogInst = inst.Name
		m.textInput.SetValue("")
		m.textInput.Placeholder = "session-name (or empty for auto)"
		m.textInput.CharLimit = 64
		m.textInput.Focus()
		return m, m.textInput.Cursor.BlinkCmd()

	case rowSession:
		// Connect to a specific named tmux session (or session group).
		inst := r.instance
		sessionName := r.sessionName
		groupID := r.groupID
		// Record as last active so enter on the instance row reopens it.
		sessInstKey := r.fleetName + "/" + inst.Name
		m.lastActive[sessInstKey] = lastSession{sessionName: sessionName, groupID: groupID}
		if m.inHostTmux {
			if m.splitPaneID != "" && !splitOpen() {
				unbindHostSplitKeys()
				m.splitPaneID = ""
				m.splitFleet = ""
				m.splitInstance = ""
				m.splitSession = ""
				m.activeGroupID = ""
			}
			// If this is a group row and the group is active, toggle off.
			if m.splitPaneID != "" && m.splitInstance == inst.Name && groupID != "" && groupID == m.activeGroupID {
				// Save current group layout before killing.
				m.saveCurrentGroupLayout()
				killAllSplitPanes()
				unbindHostSplitKeys()
				m.splitPaneID = ""
				m.splitFleet = ""
				m.splitInstance = ""
				m.splitSession = ""
				m.activeGroupID = ""
				return m, nil
			}
			// If a different group is active, save its layout first.
			if m.splitPaneID != "" && m.activeGroupID != "" {
				m.saveCurrentGroupLayout()
				killAllSplitPanes()
			}
			// Set activeGroupID immediately so the indicator
			// updates without waiting for the async command.
			m.activeGroupID = groupID
			// Restore the group: query the inner tmux for all
			// sessions matching the group prefix and recreate panes.
			if groupID != "" && isGroupedSession(SanitizeSessionName(inst.Name), sessionName) {
				return m, m.restoreGroupCmd(r.fleetName, inst, groupID)
			}
			// Ungrouped/legacy session — open a single pane.
			cols, rows := tmuxWindowSize()
			cols = cols * 70 / 100
			cmd := m.instanceBackend(inst).ExecCommand(
				inst.WorkspaceDir,
				ShellCommandForSession(m.cfg, sessionName, cols, rows, true),
			)
			return m, splitPaneCmd(m.splitPaneID, r.fleetName, inst.Name, sessionName, groupID, cmd)
		}
		cmd := m.instanceBackend(inst).ExecCommand(
			inst.WorkspaceDir,
			ShellCommandForSession(m.cfg, sessionName, m.width, m.height, false),
		)
		banner := renderGradient(nameToBanner(inst.Name))
		banner += "\n  " + dimStyle.Render("ctrl+q/ctrl+o to detach (session persists)")
		return m, tea.ExecProcess(
			execWithBannerCmd(banner, cmd),
			func(err error) tea.Msg { return execDoneMsg{err} },
		)

	case rowInstance:
		_, inst := m.selectedInstance()
		if inst == nil {
			break
		}
		instFleetName := r.fleetName
		instKey := instFleetName + "/" + inst.Name
		// Split pane mode: open shell in a right-side tmux pane
		// instead of suspending the TUI. Toggle: if the same
		// instance is already open, close the pane.
		if m.inHostTmux {
			// Clear stale pane state if the split is no longer visible
			// (e.g. session killed or detached).
			if m.splitPaneID != "" && !splitOpen() {
				unbindHostSplitKeys()
				m.splitPaneID = ""
				m.splitFleet = ""
				m.splitInstance = ""
				m.splitSession = ""
				m.activeGroupID = ""
			}
			// Toggle: if the same instance is already open, close it.
			if m.splitPaneID != "" && m.splitInstance == inst.Name {
				m.saveCurrentGroupLayout()
				killAllSplitPanes()
				unbindHostSplitKeys()
				m.splitPaneID = ""
				m.splitFleet = ""
				m.splitInstance = ""
				m.splitSession = ""
				m.activeGroupID = ""
				return m, nil
			}
			return m, m.openInstanceSession(instFleetName, inst)
		}

		// Non-split: reuse last active session, fall back to
		// default session name. tmux new-session -A creates or
		// attaches automatically.
		sessionName := SanitizeSessionName(inst.Name)
		if last, ok := m.lastActive[instKey]; ok {
			sessionName = last.sessionName
		}
		m.lastActive[instKey] = lastSession{sessionName: sessionName}

		cmd := m.instanceBackend(inst).ExecCommand(inst.WorkspaceDir, ShellCommandForSession(m.cfg, sessionName, m.width, m.height, false))

		banner := renderGradient(nameToBanner(inst.Name))
		banner += "\n  " + dimStyle.Render("ctrl+q/ctrl+o to detach (session persists)")
		return m, tea.ExecProcess(
			execWithBannerCmd(banner, cmd),
			func(err error) tea.Msg { return execDoneMsg{err} },
		)
	}

	return m, nil
}

// contextualHelpKeys returns the help bar entries relevant to the
// currently selected row and its state.
func (m model) contextualHelpKeys() []string {
	r := m.currentRow()
	if r == nil {
		return []string{"n: new fleet", "q: quit"}
	}

	switch r.kind {
	case rowFleetHeader:
		keys := []string{
			"j/k: navigate", "space: expand/collapse", "enter/e: toggle",
			"a: add instance", "n: new fleet", "d: delete fleet", "r: refresh", "q: quit",
		}
		return keys

	case rowInstance:
		keys := []string{"j/k: navigate"}
		if r.instance != nil {
			switch {
			case r.instance.Status == fleet.StatusRunning:
				keys = append(keys,
					"space: show sessions", "enter/e: open shell",
					"s: stop", "a: new session", "d: delete", "t: tag",
					"f: port-forward", "c: code", "o: terminal", "l: logs",
					"r: refresh", "q: quit",
				)
			case r.instance.Status == fleet.StatusStopped:
				keys = append(keys,
					"enter/e: open shell", "s: start",
					"a: new session", "d: delete", "t: tag", "r: refresh", "q: quit",
				)
			case r.instance.Status == fleet.StatusFailed:
				keys = append(keys, "d: delete", "r: refresh", "q: quit")
			case isTransitional(r.instance.Status):
				keys = append(keys, "r: refresh", "q: quit")
			default:
				keys = append(keys, "r: refresh", "q: quit")
			}
		}
		return keys

	case rowSession:
		keys := []string{
			"j/k: navigate", "space/enter/e: connect",
			"a: new session", "d: delete session", "r: rename", "q: quit",
		}
		if m.inHostTmux && m.splitInstance != "" && m.activeGroupID != "" {
			keys = append(keys[:len(keys)-1], "pgup/pgdn: cycle groups", "q: quit")
		}
		return keys

	case rowNewSession:
		return []string{
			"j/k: navigate", "space/enter/e: create session",
			"a: new session", "q: quit",
		}

	case rowSettings:
		return []string{
			"j/k: navigate", "space/enter/e: open settings",
			"n: new fleet", "q: quit",
		}
	}

	return []string{"q: quit"}
}

func (m model) viewFleetList() string {
	var b strings.Builder

	logo := "" +
		"  __ _         _\n" +
		" / _| |___ ___| |_\n" +
		"|  _| / -_) -_)  _|\n" +
		"|_| |_\\___\\___|\\___|"
	b.WriteString(renderGradient(logo))
	if version.Version != "" {
		b.WriteString(" " + dimStyle.Render(version.Version))
	}
	if m.updateAvailable != "" {
		b.WriteString("  " + updateStyle.Render(fmt.Sprintf("A new version: %s is available ⚡ Settings to update", m.updateAvailable)))
	}
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	}

	// Build the list content
	var listContent strings.Builder

	if m.st == nil || len(m.st.Fleets) == 0 {
		listContent.WriteString(dimStyle.Render("  No instances. Press 'a' to create one, or use 'fleet up <name>'."))
		listContent.WriteString("\n")
	}

	for i, r := range m.rows {
		isSelected := i == m.cursor
		cursor := "  "
		if isSelected {
			cursor = cursorStyle.Render("> ")
		}

		if r.kind == rowFleetHeader {
			arrow := "▼ "
			style := fleetExpandedStyle
			if m.collapsed[r.fleetName] {
				arrow = "▶ "
				style = fleetCollapsedStyle
			}

			count := 0
			if f, ok := m.st.Fleets[r.fleetName]; ok {
				count = len(f.Instances)
			}
			suffix := dimStyle.Render(fmt.Sprintf(" (%d)", count))

			if isSelected {
				listContent.WriteString(fmt.Sprintf("%s%s%s",
					cursor,
					selectedStyle.Render(arrow+r.fleetName),
					suffix,
				))
			} else {
				listContent.WriteString(fmt.Sprintf("%s%s%s%s",
					cursor,
					style.Render(arrow),
					style.Render(r.fleetName),
					suffix,
				))
			}
			listContent.WriteString("\n")
		} else if r.kind == rowSession {
			// Session row: indented under its parent instance.
			// Grouped sessions show the group ID and pane count.
			icon := "○"
			style := sessionStyle
			// Use pending group during debounce, otherwise active group.
			displayGroupID := m.activeGroupID
			if m.pendingGroupID != "" {
				displayGroupID = m.pendingGroupID
			}
			if r.groupID != "" && r.groupID == displayGroupID {
				icon = "●"
				style = sessionActiveStyle
			}
			var label string
			if r.groupSize > 1 {
				label = fmt.Sprintf("%s %s (%d panes)", icon, r.groupID, r.groupSize)
			} else if r.groupID != "" && isGroupedSession(SanitizeSessionName(r.instance.Name), r.sessionName) {
				label = fmt.Sprintf("%s %s", icon, r.groupID)
			} else {
				label = fmt.Sprintf("%s %s", icon, r.sessionName)
			}
			if isSelected {
				listContent.WriteString(fmt.Sprintf("%s      %s", cursor, selectedStyle.Render(label)))
			} else {
				listContent.WriteString(fmt.Sprintf("%s      %s", cursor, style.Render(label)))
			}
			listContent.WriteString("\n")

		} else if r.kind == rowNewSession {
			// "+" row for creating new sessions
			label := "+ new session"
			if isSelected {
				listContent.WriteString(fmt.Sprintf("%s      %s", cursor, selectedStyle.Render(label)))
			} else {
				listContent.WriteString(fmt.Sprintf("%s      %s", cursor, newSessionStyle.Render(label)))
			}
			listContent.WriteString("\n")

		} else if r.kind == rowInstance {
			inst := r.instance

			transitional := isTransitional(inst.Status)
			var status string
			if transitional {
				status = strings.TrimRight(m.spinner.View(), "\n") + " " + statusCreatingStyle.Render(string(inst.Status))
			} else {
				status = renderStatus(inst.Status)
			}

			// Show expand/collapse arrow for running instances
			instKey := r.fleetName + "/" + inst.Name
			arrow := "  "
			if inst.Status == fleet.StatusRunning {
				if m.expandedInstances[instKey] {
					arrow = "▼ "
				} else {
					arrow = "▶ "
				}
			}

			// Pad name to fixed width before styling to keep columns aligned
			paddedName := fmt.Sprintf("%s%-22s", arrow, inst.Name)
			if isSelected {
				paddedName = selectedStyle.Render(paddedName)
			}

			backendIcon := "⬡" // devcontainer
			switch inst.Backend {
			case fleet.BackendCoder:
				backendIcon = "⌨"
			case fleet.BackendCodespaces:
				backendIcon = "⏣"
			}
			branchItem := ""
			if branch := resolveWorkspaceBranch(inst.WorkspaceDir); branch != "" {
				branchItem = dimStyle.Render("  " + branch + " " + backendIcon)
			} else {
				branchItem = dimStyle.Render("  " + backendIcon)
			}

			var line string
			if transitional {
				line = fmt.Sprintf("%s    %s %s%s",
					cursor, paddedName, status, branchItem,
				)
			} else {
				// Show agent tool indicator
				agentStr := ""
				if inst.Status == fleet.StatusRunning && m.activity != nil {
					tool := m.activity.Tool(inst.ContainerID)
					label := agentToolLabel(tool)
					switch m.activity.State(inst.ContainerID) {
					case agentWorking:
						agentStr = agentWorkingStyle.Render(fmt.Sprintf("  \u25b6 %s", label))
					case agentWaiting:
						agentStr = agentWaitingStyle.Render(fmt.Sprintf("  \u23f8 %s", label))
					default:
						agentStr = agentOffStyle.Render("  \u25cb idle")
					}
				}

				// Show CPU/memory stats
				statsStr := ""
				if s, ok := m.stats[inst.ContainerID]; ok {
					statsStr = dimStyle.Render(fmt.Sprintf("  %4.0f mcpu  %6.1f MB", s.CPUMillicores, s.MemoryMB))
				}

				line = fmt.Sprintf("%s    %s %s%s%s%s",
					cursor, paddedName, status, agentStr, statsStr, branchItem,
				)

				// Show active port forwards
				pfKey := r.fleetName + "/" + inst.Name
				if pfLabel := m.portForwards.FormatLabels(pfKey); pfLabel != "" {
					line += portForwardStyle.Render("  ⇄ " + pfLabel)
				}

				if inst.Tag != "" {
					line += dimStyle.Render("  # " + inst.Tag)
				}
			}

			// Truncate with ellipsis if the line exceeds the available
			// width (border + padding = 4 characters).
			if maxW := m.width - 4; maxW > 0 && lipgloss.Width(line) > maxW {
				line = ansi.Truncate(line, maxW-1, "…")
			}

			listContent.WriteString(line)
			listContent.WriteString("\n")
		} else {
			label := "settings"
			if isSelected {
				listContent.WriteString(fmt.Sprintf("%s%s", cursor, selectedStyle.Render(label)))
			} else {
				listContent.WriteString(fmt.Sprintf("%s%s", cursor, dimStyle.Render(label)))
			}
			listContent.WriteString("\n")
		}
	}

	// Wrap in a bordered box
	boxContent := strings.TrimRight(listContent.String(), "\n")
	box := listBox
	if m.width > 0 {
		// Account for border (1 char each side) and padding (1 char each side)
		box = box.Width(m.width - 2)
	}
	b.WriteString(box.Render(boxContent))
	b.WriteString("\n")

	// Totals
	var totalCPU float64
	var totalMem float64
	for _, s := range m.stats {
		totalCPU += s.CPUMillicores
		totalMem += s.MemoryMB
	}
	if len(m.stats) > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  Total: %.0f mcpu  %.1f MB", totalCPU, totalMem)))
		b.WriteString("\n")
	}

	// Dialog overlay
	switch m.mode {
	case viewConfirmDelete:
		b.WriteString("\n")
		var title, body string
		if m.dialogInst == "" {
			count := 0
			if f, ok := m.st.Fleets[m.dialogFleet]; ok {
				count = len(f.Instances)
			}
			title = "Delete fleet"
			body = fmt.Sprintf("Remove fleet %s and all %d instance(s)? This will stop all containers and delete all workspaces.", m.dialogFleet, count)
		} else {
			title = "Delete instance"
			body = fmt.Sprintf("Remove %s/%s? This will stop the container and delete the workspace.", m.dialogFleet, m.dialogInst)
		}
		dialog := fmt.Sprintf(
			"%s\n\n%s\n\n%s",
			dialogTitle.Render(title),
			dialogLabel.Render(body),
			dialogHint.Render("[y] Yes  [n] No"),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")

	case viewConfirmDeleteFleetWarn:
		b.WriteString("\n")
		count := 0
		if f, ok := m.st.Fleets[m.dialogFleet]; ok {
			count = len(f.Instances)
		}
		warnDialog := fmt.Sprintf(
			"%s\n\n%s\n\n%s\n\n%s",
			warnBanner.Render("  !! WARNING !!  "),
			dialogLabel.Render(fmt.Sprintf(
				"You are about to destroy fleet %s with %d running instance(s).\nAll containers will be stopped and all workspace data will be permanently deleted.",
				m.dialogFleet, count,
			)),
			errorStyle.Render("This action cannot be undone."),
			dialogHint.Render("[y] Confirm destroy  [n] Cancel"),
		)
		b.WriteString(warnBox.Render(warnDialog))
		b.WriteString("\n")

	case viewAddInstance:
		b.WriteString("\n")
		bt := m.dialogBackend
		if bt == "" {
			bt = fleet.BackendDevcontainer
		}
		hint := "[enter] Create  [esc] Cancel"
		if len(m.availableBackendTypes()) > 1 {
			hint = "[enter] Create  [tab] Change deploy target  [esc] Cancel"
		}
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n%s %s\n%s [ %s ]\n\n%s",
			dialogTitle.Render("New instance"),
			dialogLabel.Render("Fleet:  "),
			fleetExpandedStyle.Render(m.dialogFleet),
			dialogLabel.Render("Name:   "),
			m.textInput.View(),
			dialogLabel.Render("Deploy: "),
			backendTypeLabel(bt),
			dialogHint.Render(hint),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")

	case viewAddFleet:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n\n%s",
			dialogTitle.Render("New fleet"),
			dialogLabel.Render("Repo:"),
			m.textInput.View(),
			dialogHint.Render("[enter] Add  [esc] Cancel"),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")

	case viewTagInstance:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n%s %s\n\n%s",
			dialogTitle.Render("Tag instance"),
			dialogLabel.Render("Instance:"),
			fleetExpandedStyle.Render(m.dialogFleet+"/"+m.dialogInst),
			dialogLabel.Render("Tag:     "),
			m.textInput.View(),
			dialogHint.Render("[enter] Save  [esc] Cancel"),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")

	case viewPortForward:
		b.WriteString("\n")
		pfKey := m.dialogFleet + "/" + m.dialogInst
		fwds := m.portForwards.List(pfKey)

		var fwdLines strings.Builder
		if len(fwds) == 0 {
			fwdLines.WriteString(dimStyle.Render("  No active forwards"))
		} else {
			for i, f := range fwds {
				cursor := "  "
				if i == m.pfCursor {
					cursor = cursorStyle.Render("> ")
				}
				fwdLines.WriteString(fmt.Sprintf("%s%s\n",
					cursor,
					portForwardStyle.Render(f.Label()),
				))
			}
		}

		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n\n%s\n\n%s %s\n\n%s",
			dialogTitle.Render("Port forwards"),
			dialogLabel.Render("Instance:"),
			fleetExpandedStyle.Render(m.dialogFleet+"/"+m.dialogInst),
			strings.TrimRight(fwdLines.String(), "\n"),
			dialogLabel.Render("Add:"),
			m.textInput.View(),
			dialogHint.Render("[enter] Add  [d] Delete selected  [j/k] Navigate  [esc] Close"),
		)
		b.WriteString(portForwardBox.Render(dialog))
		b.WriteString("\n")

	case viewCodespacesAuth:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s\n\n%s\n\n%s",
			warnBanner.Render("  GitHub Auth Required  "),
			dialogLabel.Render(
				"GitHub CLI authentication with the \"codespace\" scope is\n"+
					"required. Press Enter to log in and grant the required scope.",
			),
			dimStyle.Render("gh auth login -h github.com -s codespace"),
			dialogHint.Render("[enter] Authenticate  [esc] Cancel"),
		)
		b.WriteString(warnBox.Render(dialog))
		b.WriteString("\n")

	case viewCodespacesMachine:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s\n\n%s",
			warnBanner.Render("  Machine Type Required  "),
			dialogLabel.Render(
				"GitHub Codespaces requires a machine type but none is\n"+
					"configured. Press Enter to open Settings and set one.",
			),
			dialogHint.Render("[enter] Open Settings  [esc] Cancel"),
		)
		b.WriteString(warnBox.Render(dialog))
		b.WriteString("\n")

	case viewCodespacesLimit:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s\n\n%s",
			warnBanner.Render("  Codespace Limit Reached  "),
			dialogLabel.Render(
				"You have started the maximum number of Codespaces.\n"+
					"Please stop some before creating a new instance,\n"+
					"or use a different instance backend.",
			),
			dialogHint.Render("[enter/esc] Dismiss"),
		)
		b.WriteString(warnBox.Render(dialog))
		b.WriteString("\n")

	case viewCreateSession:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n%s %s\n\n%s",
			dialogTitle.Render("New session"),
			dialogLabel.Render("Instance:"),
			fleetExpandedStyle.Render(m.dialogFleet+"/"+m.dialogInst),
			dialogLabel.Render("Name:    "),
			m.textInput.View(),
			dialogHint.Render("[enter] Create (empty for auto-name)  [esc] Cancel"),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")

	case viewRenameSession:
		b.WriteString("\n")
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n%s %s\n%s %s\n\n%s",
			dialogTitle.Render("Rename session"),
			dialogLabel.Render("Instance:"),
			fleetExpandedStyle.Render(m.dialogFleet+"/"+m.dialogInst),
			dialogLabel.Render("Current: "),
			sessionStyle.Render(m.dialogSession),
			dialogLabel.Render("New:     "),
			m.textInput.View(),
			dialogHint.Render("[enter] Rename  [esc] Cancel"),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")

	case viewConfirmDeleteSession:
		b.WriteString("\n")
		displayName := m.dialogSession
		if m.dialogGroupID != "" {
			displayName = m.dialogGroupID
		}
		dialog := fmt.Sprintf(
			"%s\n\n%s\n\n%s",
			dialogTitle.Render("Delete session"),
			dialogLabel.Render(fmt.Sprintf("Remove session %s from %s/%s?",
				displayName, m.dialogFleet, m.dialogInst)),
			dialogHint.Render("[y] Yes  [n] No"),
		)
		b.WriteString(dialogBox.Render(dialog))
		b.WriteString("\n")
	}

	// Message
	if m.message != "" {
		b.WriteString(messageStyle.Render(m.message))
		b.WriteString("\n")
	}

	if m.cfg == nil || m.cfg.GeneralSettings.ShowHelpTextEnabled() {
		b.WriteString(renderHelp(m.width, m.contextualHelpKeys()))
	}

	return b.String()
}

// openInstanceSession opens a split pane for the given instance, reusing
// the last active session when available. The priority is:
//  1. Last active session (from in-memory tracking).
//  2. First discovered session (if the instance was expanded).
//  3. Brand new session group (fallback).
func (m model) openInstanceSession(fleetName string, inst *fleet.Instance) tea.Cmd {
	instKey := fleetName + "/" + inst.Name
	sanitized := SanitizeSessionName(inst.Name)

	// 1. Reopen the last active session for this instance.
	if last, ok := m.lastActive[instKey]; ok {
		if last.groupID != "" {
			return m.restoreGroupCmd(fleetName, inst, last.groupID)
		}
		cols, rows := tmuxWindowSize()
		cols = cols * 70 / 100
		cmd := m.instanceBackend(inst).ExecCommand(
			inst.WorkspaceDir,
			ShellCommandForSession(m.cfg, last.sessionName, cols, rows, true),
		)
		return splitPaneCmd(m.splitPaneID, fleetName, inst.Name, last.sessionName, last.groupID, cmd)
	}

	// 2. Use the first discovered session (if available).
	if disc, ok := m.sessions[instKey]; ok && disc.err == nil && len(disc.sessions) > 0 {
		groups := groupSessions(sanitized, disc.sessions)
		if len(groups) > 0 {
			g := groups[0]
			rootName := g.Sessions[0].Name
			if g.GroupID != "" && isGroupedSession(sanitized, rootName) {
				return m.restoreGroupCmd(fleetName, inst, g.GroupID)
			}
			cols, rows := tmuxWindowSize()
			cols = cols * 70 / 100
			cmd := m.instanceBackend(inst).ExecCommand(
				inst.WorkspaceDir,
				ShellCommandForSession(m.cfg, rootName, cols, rows, true),
			)
			return splitPaneCmd(m.splitPaneID, fleetName, inst.Name, rootName, g.GroupID, cmd)
		}
	}

	// 3. No sessions exist — create a new session group.
	newGroupID := randomHex(3)
	sessName := groupSessionName(sanitized, newGroupID)
	cols, rows := tmuxWindowSize()
	cols = cols * 70 / 100
	cmd := m.instanceBackend(inst).ExecCommand(
		inst.WorkspaceDir,
		ShellCommandForSession(m.cfg, sessName, cols, rows, true),
	)
	return splitPaneCmd(m.splitPaneID, fleetName, inst.Name, sessName, newGroupID, cmd)
}

// instanceGroups returns the session groups for the given instance name,
// using the current session discovery data.
func (m *model) instanceGroups(instanceName string) []sessionGroup {
	sanitized := SanitizeSessionName(instanceName)
	for _, disc := range m.sessions {
		if disc == nil || disc.err != nil {
			continue
		}
		g := groupSessions(sanitized, disc.sessions)
		if len(g) > 0 {
			return g
		}
	}
	return nil
}

// cycleSessionGroup moves the visual selection to the next or previous
// session group and starts a 500ms debounce timer. The actual pane
// switch only happens when the timer expires without further input.
func (m model) cycleSessionGroup(prev bool) (tea.Model, tea.Cmd) {
	groups := m.instanceGroups(m.splitInstance)
	if len(groups) < 2 {
		return m, nil
	}

	// Determine which group ID we're cycling FROM: if a pending
	// selection exists, cycle from that; otherwise from the active group.
	fromID := m.activeGroupID
	if m.pendingGroupID != "" {
		fromID = m.pendingGroupID
	}

	currentIdx := -1
	for i, g := range groups {
		if g.GroupID == fromID {
			currentIdx = i
			break
		}
	}
	if currentIdx < 0 {
		return m, nil
	}

	// Calculate target index with wrapping.
	targetIdx := currentIdx - 1
	if !prev {
		targetIdx = currentIdx + 1
	}
	if targetIdx < 0 {
		targetIdx = len(groups) - 1
	} else if targetIdx >= len(groups) {
		targetIdx = 0
	}

	// Update the visual selection (pendingGroupID) and bump the
	// debounce sequence. The timer will only fire the switch if
	// the sequence hasn't changed.
	m.pendingGroupID = groups[targetIdx].GroupID
	m.debounceSeq++
	return m, groupCycleDebounce(m.debounceSeq)
}

// commitGroupCycle performs the actual pane switch after the debounce
// timer expires. Called from the groupCycleMsg handler.
func (m model) commitGroupCycle() (tea.Model, tea.Cmd) {
	if m.pendingGroupID == "" || m.pendingGroupID == m.activeGroupID {
		m.pendingGroupID = ""
		return m, nil
	}

	// Find the instance.
	var inst *fleet.Instance
	for _, f := range m.st.Fleets {
		for _, i := range f.Instances {
			if i.Name == m.splitInstance {
				inst = i
				break
			}
		}
		if inst != nil {
			break
		}
	}
	if inst == nil {
		m.pendingGroupID = ""
		return m, nil
	}

	targetGroupID := m.pendingGroupID
	m.pendingGroupID = ""

	// Save current layout BEFORE changing activeGroupID, so it's
	// stored under the old group ID (not the target).
	m.saveCurrentGroupLayout()
	killAllSplitPanes()

	// Now set activeGroupID so the indicator shows the target group
	// while restoreGroupCmd is running (~2s).
	m.activeGroupID = targetGroupID

	return m, m.restoreGroupCmd(m.splitFleet, inst, targetGroupID)
}
