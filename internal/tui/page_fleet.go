package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/gitutil"
	"github.com/BenjaminBenetti/fleet-man/internal/instanceops"
	tea "github.com/charmbracelet/bubbletea"
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
			// Toggle collapse on fleet headers
			if r := m.currentRow(); r != nil && r.kind == rowFleetHeader {
				name := r.fleetName
				m.collapsed[name] = !m.collapsed[name]
				m.buildRows()
			}

		case "r":
			m.reload()
			m.message = "Refreshed"

		case "s":
			r := m.currentRow()
			if r == nil || r.kind != rowInstance || r.instance == nil {
				m.message = "Select an instance"
				break
			}

			key := r.fleetName + "/" + r.instance.Name
			if r.instance.Status == fleet.StatusCreating {
				m.message = fmt.Sprintf("Instance %s is still creating", key)
				break
			}
			if r.instance.Status == fleet.StatusFailed {
				m.message = fmt.Sprintf("Instance %s is failed and cannot be toggled", key)
				break
			}

			result, err := toggleInstanceStatus(r.fleetName, r.instance.Name)
			if err != nil {
				m.message = fmt.Sprintf("Could not toggle %s: %v", key, err)
				break
			}

			m.reload()
			switch result.Status {
			case fleet.StatusStopped:
				if result.Changed {
					m.message = fmt.Sprintf("Stopped %s", key)
				} else {
					m.message = fmt.Sprintf("Instance %s is already stopped", key)
				}
			case fleet.StatusRunning:
				if result.Changed {
					m.message = fmt.Sprintf("Started %s", key)
				} else {
					m.message = fmt.Sprintf("Instance %s is already running", key)
				}
			default:
				m.message = fmt.Sprintf("Instance %s is %s", key, result.Status)
			}

		case "d":
			r := m.currentRow()
			if r == nil || r.kind == rowSettings {
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
			fleetName := m.currentFleetName()
			if fleetName == "" {
				m.message = "No fleet selected"
				break
			}
			m.mode = viewAddInstance
			m.dialogFleet = fleetName
			m.dialogBackend = fleet.BackendDevcontainer
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

		case "enter", "e":
			r := m.currentRow()
			if r == nil {
				break
			}
			if r.kind == rowSettings {
				m.page = pageSettings
				return m, nil
			}

			_, inst := m.selectedInstance()
			if inst == nil {
				// If on a fleet header, toggle collapse
				if r.kind == rowFleetHeader {
					name := r.fleetName
					m.collapsed[name] = !m.collapsed[name]
					m.buildRows()
				}
				break
			}
			banner := renderGradient(nameToBanner(inst.Name))
			banner += "\n  " + dimStyle.Render("ctrl+q/ctrl+o to detach (session persists)")
			cmd := m.instanceBackend(inst).ExecCommand(inst.WorkspaceDir, shellCommand(m.cfg, inst.Name, m.width, m.height))
			return m, tea.ExecProcess(
				execWithBannerCmd(banner, cmd),
				func(err error) tea.Msg { return execDoneMsg{err} },
			)

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
			uri, ok := m.instanceBackend(inst).EditorURI(inst.WorkspaceDir, r.fleetName)
			if !ok {
				m.message = "Editor integration not supported by this backend"
				break
			}
			codeCmd := exec.Command("code", "--folder-uri", uri)
			if err := codeCmd.Run(); err != nil {
				m.message = fmt.Sprintf("VS Code error: %v", err)
			} else {
				m.message = fmt.Sprintf("Opened VS Code for %s", inst.Name)
			}

		case "l":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			return m, tea.ExecProcess(
				m.instanceBackend(inst).LogsCommand(inst.ContainerID, false),
				func(err error) tea.Msg { return execDoneMsg{err} },
			)
		}

	case execDoneMsg:
		m.reload()
		if msg.err != nil {
			m.message = fmt.Sprintf("Command error: %v", msg.err)
		}
	}

	return m, nil
}

func (m model) viewFleetList() string {
	var b strings.Builder

	logo := "" +
		"  __ _         _\n" +
		" / _| |___ ___| |_\n" +
		"|  _| / -_) -_)  _|\n" +
		"|_| |_\\___\\___|\\___|"
	b.WriteString(renderGradient(logo))
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
		} else if r.kind == rowInstance {
			inst := r.instance
			key := r.fleetName + "/" + inst.Name

			var status string
			if m.creating[key] {
				status = m.spinner.View() + " " + statusCreatingStyle.Render("creating")
			} else {
				status = renderStatus(inst.Status)
			}

			// Pad name to fixed width before styling to keep columns aligned
			paddedName := fmt.Sprintf("%-24s", inst.Name)
			if isSelected {
				paddedName = selectedStyle.Render(paddedName)
			}

			branchItem := ""
			if branch := resolveWorkspaceBranch(inst.WorkspaceDir); branch != "" {
				branchItem = dimStyle.Render("  " + branch)
			}

			if m.creating[key] {
				listContent.WriteString(fmt.Sprintf("%s    %s %s",
					cursor, paddedName, status,
				))
				listContent.WriteString(branchItem)
			} else {
				// Show agent tool indicator
				agentStr := ""
				if inst.Status == fleet.StatusRunning && m.activity != nil {
					// Use detected tool if available, fall back to configured tool
					tool := m.activity.Tool(inst.ContainerID)
					if tool == "" && m.cfg != nil {
						tool = m.cfg.AgentSettings.ToolSelection
					}
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
				listContent.WriteString(fmt.Sprintf("%s    %s %s%s%s",
					cursor, paddedName, status, agentStr, statsStr,
				))
				listContent.WriteString(branchItem)
			}
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
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n%s %s\n%s [ %s ]\n\n%s",
			dialogTitle.Render("New instance"),
			dialogLabel.Render("Fleet:  "),
			fleetExpandedStyle.Render(m.dialogFleet),
			dialogLabel.Render("Name:   "),
			m.textInput.View(),
			dialogLabel.Render("Deploy: "),
			backendTypeLabel(bt),
			dialogHint.Render("[enter] Create  [left/right] Change deploy target  [esc] Cancel"),
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
	}

	// Message
	if m.message != "" {
		b.WriteString(messageStyle.Render(m.message))
		b.WriteString("\n")
	}

	b.WriteString(renderHelp(m.width, []string{
		"j/k: navigate", "space: expand/collapse", "enter/e: exec or open",
		"s: stop/start", "o: open terminal", "n: new fleet", "a: add instance", "d: delete",
		"c: code", "l: logs", "r: refresh", "q: quit",
	}))

	return b.String()
}
