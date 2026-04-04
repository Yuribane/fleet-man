package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	"github.com/BenjaminBenetti/fleet-man/internal/backendutil"
	"github.com/BenjaminBenetti/fleet-man/internal/deps"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewMode int

const (
	viewNormal viewMode = iota
	viewConfirmDelete
	viewConfirmDeleteFleetWarn
	viewAddInstance
	viewAddFleet
	viewTagInstance
	viewDepsCheck
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

	page          pageMode
	mode          viewMode
	dialogFleet   string
	dialogInst    string
	dialogBackend fleet.BackendType // selected backend in add-instance dialog
	textInput     textinput.Model

	spinner  spinner.Model
	creating map[string]bool // "fleet/instance" keys currently being created

	backends map[fleet.BackendType]backend.Backend // one per backend type, lazily created
	stats    map[string]*backend.ContainerStats    // containerID → stats
	activity *ActivityTracker                      // agent working/waiting/idle detection

	settingsCursor  int             // index into settings rows
	settingsEditing bool            // true when editing a text field
	settingsInput   textinput.Model // dedicated text input for settings page

	coderPresets       []string // available preset names (in-memory, from API)
	coderFetchingParams bool    // true while fetching template parameters

	depsResult []deps.Dependency // set on first-ever startup

	// Split pane mode: when fleet runs inside a host tmux session,
	// pressing enter opens the instance shell in a right-side pane
	// instead of suspending the TUI.
	inHostTmux    bool   // true when TMUX env var is set at startup
	splitPaneID   string // tmux pane ID of the right pane ("" = no split)
	splitInstance string // instance name currently in the right pane

	message  string
	quitting bool
	width    int
	height   int
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
		backends:      make(map[fleet.BackendType]backend.Backend),
		stats:         make(map[string]*backend.ContainerStats),
		activity:      NewActivityTracker(),
		textInput:     ti,
		spinner:       sp,
		settingsInput: si,
		inHostTmux:    os.Getenv("TMUX") != "",
	}
	// On first-ever startup, check for required binaries and show results
	// if anything is missing. "First startup" = the ~/.fleet/ dir doesn't exist.
	if _, err := os.Stat(state.FleetDir()); os.IsNotExist(err) {
		result := deps.Check()
		if deps.HasMissing(result) {
			m.depsResult = result
			m.mode = viewDepsCheck
		}
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

// backendFor returns the cached backend for the given type, creating it lazily.
func (m *model) backendFor(bt fleet.BackendType) backend.Backend {
	if bt == "" {
		bt = fleet.BackendDevcontainer
	}
	if b, ok := m.backends[bt]; ok {
		return b
	}
	b := backendutil.New(bt, false)
	m.backends[bt] = b
	return b
}

// instanceBackend returns the backend for the given instance's backend type.
func (m *model) instanceBackend(inst *fleet.Instance) backend.Backend {
	return m.backendFor(inst.Backend)
}

// backendGroup holds container IDs and sessions grouped by backend type.
type backendGroup struct {
	ids      []string
	sessions map[string]string
}

// containersByBackend groups running instances by their backend type.
func (m *model) containersByBackend() map[fleet.BackendType]*backendGroup {
	groups := make(map[fleet.BackendType]*backendGroup)
	for _, f := range m.st.Fleets {
		for _, inst := range f.Instances {
			if inst.ContainerID == "" || inst.Status != fleet.StatusRunning {
				continue
			}
			bt := inst.Backend
			if bt == "" {
				bt = fleet.BackendDevcontainer
			}
			g, ok := groups[bt]
			if !ok {
				g = &backendGroup{sessions: make(map[string]string)}
				groups[bt] = g
			}
			g.ids = append(g.ids, inst.ContainerID)
			g.sessions[inst.ContainerID] = sanitizeSessionName(inst.Name)
		}
	}
	return groups
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, m.fetchAllStatsCmd(false)}
	if len(m.creating) > 0 {
		cmds = append(cmds, pollCreatingCmd())
	}
	// Auto-fetch coder template parameters if template is configured
	if m.cfg != nil && m.cfg.CoderSettings.Template != "" {
		m.coderFetchingParams = true
		cmds = append(cmds, fetchCoderParamsCmd(m.cfg.CoderSettings.Template))
	}
	return tea.Batch(cmds...)
}

// fetchAllStatsCmd creates a command that fetches stats from all backends concurrently.
func (m model) fetchAllStatsCmd(delay bool) tea.Cmd {
	groups := m.containersByBackend()
	if len(groups) == 0 {
		return fetchStatsCmd(nil, nil, nil, delay)
	}

	type fetchInput struct {
		dc       backend.Backend
		ids      []string
		sessions map[string]string
	}
	var inputs []fetchInput
	for bt, g := range groups {
		inputs = append(inputs, fetchInput{
			dc:       m.backendFor(bt),
			ids:      g.ids,
			sessions: g.sessions,
		})
	}

	// If only one backend type, use the simple path
	if len(inputs) == 1 {
		return fetchStatsCmd(inputs[0].dc, inputs[0].ids, inputs[0].sessions, delay)
	}

	// Multiple backend types: fetch concurrently and merge
	return func() tea.Msg {
		if delay {
			time.Sleep(3 * time.Second)
		}

		allStats := make(map[string]*backend.ContainerStats)
		allScreens := make(map[string]backend.ScreenCapture)
		allProbes := make(map[string]string)
		var allIDs []string

		type result struct {
			stats   map[string]*backend.ContainerStats
			screens map[string]backend.ScreenCapture
			probes  map[string]string
			ids     []string
		}

		ch := make(chan result, len(inputs))
		for _, inp := range inputs {
			go func(dc backend.Backend, ids []string, sessions map[string]string) {
				stats, _ := dc.Stats(ids)
				screens := backend.CaptureScreens(dc, sessions)
				probes := backend.AgentToolProbes(dc, ids)
				ch <- result{stats, screens, probes, ids}
			}(inp.dc, inp.ids, inp.sessions)
		}

		for range inputs {
			r := <-ch
			for k, v := range r.stats {
				allStats[k] = v
			}
			for k, v := range r.screens {
				allScreens[k] = v
			}
			for k, v := range r.probes {
				allProbes[k] = v
			}
			allIDs = append(allIDs, r.ids...)
		}

		return statsMsg{stats: allStats, screens: allScreens, probes: allProbes, containerIDs: allIDs}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
	}

	// Handle background results
	switch msg := msg.(type) {
	case statsMsg:
		if msg.stats != nil {
			m.stats = msg.stats
		}
		if msg.screens != nil {
			m.activity.Update(msg.screens, msg.probes, msg.containerIDs, time.Now())
		}
		return m, m.fetchAllStatsCmd(true)

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

	case splitPaneMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Split pane error: %v", msg.err)
		} else {
			m.splitPaneID = msg.paneID
			m.splitInstance = msg.instance
		}
		return m, nil

	case operationDoneMsg:
		m.reload()
		if msg.err != nil {
			m.message = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.message = msg.message
		}
		return m, nil

	case coderParamsFetchedMsg:
		m.coderFetchingParams = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to fetch parameters: %v", msg.err)
			return m, nil
		}
		if m.cfg == nil {
			m.cfg = state.DefaultConfig()
		}

		// Merge parameters: keep existing user-set values, add new ones with defaults
		existing := make(map[string]string)
		for _, p := range m.cfg.CoderSettings.Parameters {
			if p.Value != "" {
				existing[p.Name] = p.Value
			}
		}
		var newParams []state.CoderParameter
		for _, rp := range msg.params {
			val := existing[rp.Name] // preserve user value if set
			newParams = append(newParams, state.CoderParameter{
				Name:         rp.Name,
				Value:        val,
				DefaultValue: rp.DefaultValue,
				DisplayName:  rp.DisplayName,
				Description:  rp.Description,
				Type:         rp.Type,
			})
		}
		m.cfg.CoderSettings.Parameters = newParams

		// Collect preset names
		m.coderPresets = nil
		for _, p := range msg.presets {
			m.coderPresets = append(m.coderPresets, p.Name)
		}
		// If no preset selected yet and presets exist, select the first one
		if m.cfg.CoderSettings.Preset == "" && len(m.coderPresets) > 0 {
			m.cfg.CoderSettings.Preset = m.coderPresets[0]
		}

		_ = state.SaveConfig(m.cfg)
		m.message = fmt.Sprintf("Loaded %d parameters, %d presets", len(newParams), len(m.coderPresets))
		return m, nil

	case pollCreatingTickMsg:
		if len(m.creating) == 0 {
			return m, nil
		}
		m.reload()
		var cmds []tea.Cmd
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
			cmds = append(cmds, pollCreatingCmd())
		}
		return m, tea.Batch(cmds...)
	}

	// Always update spinner and batch its tick cmd with the mode cmd
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)

	modeModel, modeCmd := m.updateByMode(msg)
	m = modeModel.(model)
	return m, tea.Batch(spinCmd, modeCmd)
}

func (m model) updateByMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode == viewDepsCheck {
		return m.updateDepsCheck(msg)
	}

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
	case viewTagInstance:
		return m.updateTagInstance(msg)
	default:
		return m.updateNormal(msg)
	}
}

func (m model) View() string {
	if m.quitting {
		killSplitPane(m.splitPaneID)
		return ""
	}

	if m.mode == viewDepsCheck {
		return m.viewDepsCheck()
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
