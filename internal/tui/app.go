package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/fleet-man/fleet-man/internal/devcontainer"
	"github.com/fleet-man/fleet-man/internal/fleet"
	"github.com/fleet-man/fleet-man/internal/state"
)

// row represents a single navigable row in the TUI — either a fleet header or an instance.
type row struct {
	fleetName    string
	instance     *fleet.Instance
	isFleetHeader bool
}

type model struct {
	rows     []row
	cursor   int
	st       *state.State
	err      error
	message  string
	quitting bool
}

func newModel() model {
	m := model{}
	m.reload()
	return m
}

func (m *model) reload() {
	st, err := state.Load()
	if err != nil {
		m.err = err
		return
	}
	m.st = st
	m.err = nil
	m.buildRows()
}

func (m *model) buildRows() {
	m.rows = nil
	for name, f := range m.st.Fleets {
		m.rows = append(m.rows, row{fleetName: name, isFleetHeader: true})
		for _, inst := range f.Instances {
			m.rows = append(m.rows, row{fleetName: name, instance: inst})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m *model) selectedInstance() (*fleet.Fleet, *fleet.Instance) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil, nil
	}
	r := m.rows[m.cursor]
	if r.isFleetHeader || r.instance == nil {
		return nil, nil
	}
	f := m.st.Fleets[r.fleetName]
	return f, r.instance
}

// Init implements tea.Model.
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.message = ""

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}

		case "r":
			m.reload()
			m.message = "Refreshed"

		case "d":
			f, inst := m.selectedInstance()
			if inst == nil {
				m.message = "No instance selected"
				break
			}
			dc := devcontainer.NewClient()
			_ = dc.Down(inst.ContainerID)
			if inst.WorkspaceDir != "" {
				_ = os.RemoveAll(inst.WorkspaceDir)
			}
			_ = f.RemoveInstance(inst.Name)
			_ = state.Save(m.st)
			m.buildRows()
			m.message = fmt.Sprintf("Removed %s", inst.Name)

		case "enter", "e":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "No instance selected"
				break
			}
			return m, tea.ExecProcess(
				exec.Command("devcontainer", "exec", "--workspace-folder", inst.WorkspaceDir, "bash"),
				func(err error) tea.Msg { return execDoneMsg{err} },
			)

		case "c":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "No instance selected"
				break
			}
			r := m.rows[m.cursor]
			hexPath := fmt.Sprintf("%x", inst.WorkspaceDir)
			folderURI := fmt.Sprintf("vscode-remote://dev-container+%s/workspaces/%s", hexPath, r.fleetName)
			cmd := exec.Command("code", "--folder-uri", folderURI)
			if err := cmd.Run(); err != nil {
				m.message = fmt.Sprintf("VS Code error: %v", err)
			} else {
				m.message = fmt.Sprintf("Opened VS Code for %s", inst.Name)
			}

		case "l":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "No instance selected"
				break
			}
			return m, tea.ExecProcess(
				exec.Command("docker", "logs", inst.ContainerID),
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

type execDoneMsg struct{ err error }

// View implements tea.Model.
func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("fleet — devcontainer fleet manager"))
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	}

	if len(m.rows) == 0 {
		b.WriteString(dimStyle.Render("No instances. Use 'fleet up <name>' to create one."))
		b.WriteString("\n")
	}

	for i, r := range m.rows {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		if r.isFleetHeader {
			line := fmt.Sprintf("%s%s", cursor, headerStyle.Render(r.fleetName))
			if i == m.cursor {
				line = selectedStyle.Render(fmt.Sprintf("> %s", r.fleetName))
			}
			b.WriteString(line)
			b.WriteString("\n")
		} else {
			inst := r.instance
			status := renderStatus(inst.Status)
			containerShort := inst.ContainerID
			if len(containerShort) > 12 {
				containerShort = containerShort[:12]
			}

			line := fmt.Sprintf("%s  %-20s %s  %s", cursor, inst.Name, status, dimStyle.Render(containerShort))
			if i == m.cursor {
				line = fmt.Sprintf("%s  %s %s  %s",
					selectedStyle.Render(">"),
					selectedStyle.Render(fmt.Sprintf("%-20s", inst.Name)),
					status,
					dimStyle.Render(containerShort),
				)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if m.message != "" {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(m.message))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("j/k: navigate • enter/e: exec bash • c: VS Code • l: logs • d: remove • r: refresh • q: quit"))
	b.WriteString("\n")

	return b.String()
}

func renderStatus(s fleet.InstanceStatus) string {
	switch s {
	case fleet.StatusRunning:
		return statusRunning.Render("running")
	case fleet.StatusStopped:
		return statusStopped.Render("stopped")
	case fleet.StatusCreating:
		return statusCreating.Render("creating")
	default:
		return dimStyle.Render(string(s))
	}
}

// Run starts the TUI.
func Run() error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
