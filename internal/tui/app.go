package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type agentState int

const (
	agentNotRunning agentState = iota
	agentWorking
	agentWaiting
)

type viewMode int

const (
	viewNormal viewMode = iota
	viewConfirmDelete
	viewConfirmDeleteFleetWarn
	viewAddInstance
	viewAddFleet
)

type pageMode int

const (
	pageFleetList pageMode = iota
	pageSettings
)

type rowKind int

const (
	rowFleetHeader rowKind = iota
	rowInstance
	rowSettings
)

// row represents a single navigable row in the TUI.
type row struct {
	kind      rowKind
	fleetName string
	instance  *fleet.Instance
}

type model struct {
	rows      []row
	cursor    int
	collapsed map[string]bool

	st  *state.State
	cfg *state.Config
	err error

	page        pageMode
	mode        viewMode
	dialogFleet string
	dialogInst  string
	textInput   textinput.Model

	spinner  spinner.Model
	creating map[string]bool // "fleet/instance" keys currently being created

	stats          map[string]*devcontainer.ContainerStats // containerID → stats
	agentStates    map[string]agentState                  // containerID → derived agent state
	agentTools     map[string]state.AgentTool             // containerID → detected tool
	agentPrevTicks map[string]int64                       // containerID → previous CPU ticks for delta

	settingsCursor  int             // 0=tool, 1=repo URL, 2=install script
	settingsEditing bool            // true when editing a text field
	settingsInput   textinput.Model // dedicated text input for settings page

	message  string
	quitting bool
	width    int
}

func newModel() model {
	ti := textinput.New()
	ti.Placeholder = "instance-name"
	ti.CharLimit = 64

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))

	si := textinput.New()
	si.CharLimit = 256

	m := model{
		collapsed:     make(map[string]bool),
		creating:      make(map[string]bool),
		stats:          make(map[string]*devcontainer.ContainerStats),
		agentStates:    make(map[string]agentState),
		agentPrevTicks: make(map[string]int64),
		textInput:     ti,
		spinner:       sp,
		settingsInput: si,
	}
	m.reload()

	// Resume tracking any instances still in "creating" state from a previous session
	if m.st != nil {
		for fleetName, f := range m.st.Fleets {
			for _, inst := range f.Instances {
				if inst.Status == fleet.StatusCreating {
					m.creating[fleetName+"/"+inst.Name] = true
				}
			}
		}
	}

	return m
}

func (m *model) reload() {
	st, err := state.Load()
	if err != nil {
		m.err = err
		return
	}

	cfg, err := state.LoadConfig()
	if err != nil {
		m.err = err
		return
	}

	m.st = st
	m.cfg = cfg
	m.err = nil
	m.buildRows()
}

func (m *model) buildRows() {
	wasOnSettings := false
	if r := m.currentRow(); r != nil && r.kind == rowSettings {
		wasOnSettings = true
	}

	m.rows = nil

	// Sort fleet names for stable ordering
	names := make([]string, 0, len(m.st.Fleets))
	for name := range m.st.Fleets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		f := m.st.Fleets[name]
		m.rows = append(m.rows, row{kind: rowFleetHeader, fleetName: name})
		if !m.collapsed[name] {
			for _, inst := range f.Instances {
				m.rows = append(m.rows, row{kind: rowInstance, fleetName: name, instance: inst})
			}
		}
	}
	m.rows = append(m.rows, row{kind: rowSettings})
	if wasOnSettings {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m *model) currentRow() *row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *model) moveCursor(delta int) {
	if len(m.rows) == 0 || delta == 0 {
		return
	}

	m.cursor = (m.cursor + delta + len(m.rows)) % len(m.rows)
}

func (m *model) currentFleetName() string {
	r := m.currentRow()
	if r == nil || r.kind == rowSettings {
		return ""
	}
	return r.fleetName
}

func (m *model) selectedInstance() (*fleet.Fleet, *fleet.Instance) {
	r := m.currentRow()
	if r == nil || r.kind != rowInstance || r.instance == nil {
		return nil, nil
	}
	f := m.st.Fleets[r.fleetName]
	return f, r.instance
}

func (m *model) containerIDs() []string {
	var ids []string
	for _, f := range m.st.Fleets {
		for _, inst := range f.Instances {
			if inst.ContainerID != "" && inst.Status == fleet.StatusRunning {
				ids = append(ids, inst.ContainerID)
			}
		}
	}
	return ids
}

// agentIdleTickThreshold is the maximum CPU tick delta (between polls)
// that still counts as idle. Allows minor background activity like
// heartbeats or GC without flipping the status to working.
const agentIdleTickThreshold int64 = 10

// deriveAgentStates updates the agent state map from probe results.
// File-based probes (claude, copilot) provide a definitive state.
// Tick-based probes (codex, gemini) use CPU tick deltas.
func (m *model) deriveAgentStates(probes map[string]devcontainer.AgentProbeResult) {
	newStates := make(map[string]agentState, len(probes))
	newPrev := make(map[string]int64, len(probes))
	newTools := make(map[string]state.AgentTool, len(probes))

	for id, r := range probes {
		if r.Tool == "" {
			newStates[id] = agentNotRunning
			continue
		}

		newTools[id] = state.AgentTool(r.Tool)

		// File-based detection: definitive state
		if r.State == "working" {
			newStates[id] = agentWorking
			continue
		}
		if r.State == "waiting" {
			newStates[id] = agentWaiting
			continue
		}

		// Tick-based detection: delta comparison
		prev, hasPrev := m.agentPrevTicks[id]
		if !hasPrev || r.CPUTicks < prev {
			newStates[id] = agentWorking
		} else if r.CPUTicks-prev > agentIdleTickThreshold {
			newStates[id] = agentWorking
		} else {
			newStates[id] = agentWaiting
		}
		newPrev[id] = r.CPUTicks
	}

	m.agentStates = newStates
	m.agentPrevTicks = newPrev
	m.agentTools = newTools
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, fetchStatsCmd(m.containerIDs(), false)}
	if len(m.creating) > 0 {
		cmds = append(cmds, pollCreatingCmd())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
	}

	// Handle background results
	switch msg := msg.(type) {
	case statsMsg:
		if msg.stats != nil {
			m.stats = msg.stats
		}
		if msg.agentProbes != nil {
			m.deriveAgentStates(msg.agentProbes)
		}
		return m, fetchStatsCmd(m.containerIDs(), true)

	case instanceSpawnedMsg:
		// Background process launched; start polling for completion
		return m, pollCreatingCmd()

	case instanceCreateErrMsg:
		key := msg.fleet + "/" + msg.instance
		delete(m.creating, key)
		// Spawn itself failed; mark as failed in state
		st, _ := state.Load()
		if st != nil {
			if f, ok := st.Fleets[msg.fleet]; ok {
				if inst, err := f.GetInstance(msg.instance); err == nil {
					inst.Status = fleet.StatusFailed
					inst.Error = msg.err.Error()
					_ = state.Save(st)
				}
			}
		}
		m.reload()
		m.message = fmt.Sprintf("Failed to create %s: %v", key, msg.err)
		return m, nil

	case pollCreatingTickMsg:
		if len(m.creating) == 0 {
			return m, nil
		}
		m.reload()
		for key := range m.creating {
			parts := strings.SplitN(key, "/", 2)
			if len(parts) != 2 {
				continue
			}
			fleetName, instName := parts[0], parts[1]
			if f, ok := m.st.Fleets[fleetName]; ok {
				if inst, err := f.GetInstance(instName); err == nil {
					switch inst.Status {
					case fleet.StatusRunning:
						delete(m.creating, key)
						m.message = fmt.Sprintf("Instance %s is running (container: %s)",
							key, inst.ContainerID[:min(12, len(inst.ContainerID))])
					case fleet.StatusFailed:
						delete(m.creating, key)
						m.message = fmt.Sprintf("Failed to create %s: %s", key, inst.Error)
					}
				}
			}
		}
		if len(m.creating) > 0 {
			return m, pollCreatingCmd()
		}
		return m, nil
	}

	// Always update spinner and batch its tick cmd with the mode cmd
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)

	modeModel, modeCmd := m.updateByMode(msg)
	m = modeModel.(model)
	return m, tea.Batch(spinCmd, modeCmd)
}

func (m model) updateByMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.page == pageSettings {
		return m.updateSettings(msg)
	}

	switch m.mode {
	case viewConfirmDelete:
		return m.updateConfirmDelete(msg)
	case viewConfirmDeleteFleetWarn:
		return m.updateConfirmDeleteFleetWarn(msg)
	case viewAddInstance:
		return m.updateAddInstance(msg)
	case viewAddFleet:
		return m.updateAddFleet(msg)
	default:
		return m.updateNormal(msg)
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	if m.page == pageSettings {
		return m.viewSettings()
	}

	return m.viewFleetList()
}

// Run starts the TUI.
func Run() error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
