package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	"github.com/BenjaminBenetti/fleet-man/internal/backendutil"
	codespacesbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/codespaces"
	"github.com/BenjaminBenetti/fleet-man/internal/deps"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/portforward"
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
	viewPortForward
	viewDepsCheck
	viewCodespacesAuth
	viewCodespacesLimit
	viewCodespacesMachine
	viewCreateSession
	viewRenameSession
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
	rowSession
	rowNewSession
	rowSettings
)

// row represents a single navigable row in the TUI.
type row struct {
	kind        rowKind
	fleetName   string
	instance    *fleet.Instance
	sessionName string // set when kind == rowSession or rowNewSession
	groupID     string // set for grouped session rows
	groupSize   int    // number of sessions in the group (for display)
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

	codespaceMachines         []codespaceMachine // available machine types (from GitHub API)
	codespaceFetchingMachines bool     // true while fetching machine types

	toolStatus []deps.ToolStatus // cached tool install statuses for settings page

	depsResult []deps.Dependency // set on first-ever startup

	// Port forwarding
	portForwards      *portforward.Manager // manages active port forward processes
	pfCursor          int                  // cursor within the port forward dialog list
	pfContainerID     string               // container ID for the instance being forwarded

	// Instance expansion: which instances are expanded to show sessions
	expandedInstances map[string]bool              // key: "fleet/instance"
	sessions          map[string]*sessionDiscovery // key: "fleet/instance"
	activeSessions    map[string]string            // containerID → active tmux session name
	sessionPoller     *sessionPoller               // manages fast 1s session poll loop
	dialogSession     string                       // session being renamed (for viewRenameSession)

	// Split pane mode: when fleet runs inside a host tmux session,
	// pressing enter opens the instance shell in a right-side pane
	// instead of suspending the TUI.
	inHostTmux    bool   // true when TMUX env var is set at startup
	splitPaneID   string // tmux pane ID of the right pane ("" = no split)
	splitInstance string // instance name currently in the right pane
	splitSession  string // tmux session name in the right pane

	// Session groups: track the active group and saved layouts for
	// group switching (kill all panes → restore).
	activeGroupID string                // group ID of current outer panes
	savedGroups   map[string]savedGroup // groupID → saved layout for restoration

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
		collapsed:         make(map[string]bool),
		creating:          make(map[string]bool),
		backends:          make(map[fleet.BackendType]backend.Backend),
		stats:             make(map[string]*backend.ContainerStats),
		activity:          NewActivityTracker(),
		portForwards:      portforward.NewManager(),
		expandedInstances: make(map[string]bool),
		sessions:          make(map[string]*sessionDiscovery),
		activeSessions:    make(map[string]string),
		sessionPoller:     newSessionPoller(),
		savedGroups:       make(map[string]savedGroup),
		textInput:         ti,
		spinner:           sp,
		settingsInput:     si,
		inHostTmux:        os.Getenv("TMUX") != "",
	}
	// Unbind C-PPage/C-NPage from the host tmux so they pass through
	// to inner tmux sessions for session cycling. Bind Ctrl+Q/O to
	// close all split panes from any pane.
	if m.inHostTmux {
		unbindHostSessionKeys()
		bindHostCloseKeys()
		// Neutralize default split bindings so the user doesn't
		// accidentally open a host shell before selecting an instance.
		// These will be rebound to connect to the active instance
		// once a split pane is opened.
		unbindDefaultSplitKeys()
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

	// Auto-collapse expanded instances that are no longer running
	for key := range m.expandedInstances {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 {
			if f, ok := st.Fleets[parts[0]]; ok {
				if inst, err := f.GetInstance(parts[1]); err == nil && inst.Status == fleet.StatusRunning {
					continue
				}
			}
		}
		delete(m.expandedInstances, key)
		delete(m.sessions, key)
	}

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
				instKey := name + "/" + inst.Name
				if m.expandedInstances[instKey] {
					if disc, ok := m.sessions[instKey]; ok && disc.err == nil {
						sanitized := SanitizeSessionName(inst.Name)
						groups := groupSessions(sanitized, disc.sessions)
						for _, g := range groups {
							// Use the root session name (first in group) for display.
							rootName := g.Sessions[0].Name
							m.rows = append(m.rows, row{
								kind:        rowSession,
								fleetName:   name,
								instance:    inst,
								sessionName: rootName,
								groupID:     g.GroupID,
								groupSize:   len(g.Sessions),
							})
						}
					}
					m.rows = append(m.rows, row{
						kind:      rowNewSession,
						fleetName: name,
						instance:  inst,
					})
				}
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

// selectedSession returns the fleet, instance, and session name when
// the cursor is on a session row.
func (m *model) selectedSession() (*fleet.Fleet, *fleet.Instance, string) {
	r := m.currentRow()
	if r == nil || r.kind != rowSession {
		return nil, nil, ""
	}
	f := m.st.Fleets[r.fleetName]
	return f, r.instance, r.sessionName
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

// firstFleetRepo returns the "owner/repo" string for the first fleet's
// remote URL, or "" if no fleets exist. Used to query GitHub APIs.
func (m *model) firstFleetRepo() string {
	if m.st == nil {
		return ""
	}
	for _, f := range m.st.Fleets {
		if f.Remote != "" {
			return repoFromRemote(f.Remote)
		}
	}
	return ""
}

// instanceBackend returns the backend for the given instance's backend type.
// For codespaces, it registers the real codespace name so that exec calls
// use the correct name instead of deriving from the workspace path.
func (m *model) instanceBackend(inst *fleet.Instance) backend.Backend {
	b := m.backendFor(inst.Backend)
	if inst.Backend == fleet.BackendCodespaces && inst.ContainerID != "" {
		if csb, ok := b.(*codespacesbackend.CodespacesBackend); ok {
			csb.RegisterName(inst.WorkspaceDir, inst.ContainerID)
		}
	}
	return b
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
			g.sessions[inst.ContainerID] = SanitizeSessionName(inst.Name)
		}
	}
	return groups
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.fetchAllStatsCmd(false),
		m.sessionPollLoop(false),
	}
	if len(m.creating) > 0 {
		cmds = append(cmds, pollCreatingCmd())
	}
	// Auto-fetch coder template parameters if template is configured
	if m.cfg != nil && m.cfg.CoderSettings.Template != "" {
		m.coderFetchingParams = true
		cmds = append(cmds, fetchCoderParamsCmd(m.cfg.CoderSettings.Template))
	}
	// Auto-fetch codespace machine types from the first fleet's repo
	if repo := m.firstFleetRepo(); repo != "" {
		m.codespaceFetchingMachines = true
		cmds = append(cmds, fetchCodespaceMachinesCmd(repo))
	}
	return tea.Batch(cmds...)
}

// sessionPollLoop returns a tea.Cmd that runs the fast 1-second session
// poll loop. It polls active sessions for all running containers and
// lists sessions for expanded instances.
func (m model) sessionPollLoop(delay bool) tea.Cmd {
	return sessionPollCmd(m.sessionPoller, m.backends, m.expandedInstances, m.st.Fleets, delay)
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

	case sessionPollMsg:
		if msg.activeSessions != nil {
			m.activeSessions = msg.activeSessions
		}
		if msg.discovered != nil {
			for key, sessions := range msg.discovered {
				m.sessions[key] = &sessionDiscovery{sessions: sessions, fetchedAt: time.Now()}
			}
			m.buildRows()
		}
		// Keep the saved group layout fresh so that if Ctrl+Q/O kills
		// panes externally, the most recent layout is preserved.
		if m.activeGroupID != "" && m.splitPaneID != "" && splitOpen() {
			m.saveCurrentGroupLayout()
		}
		// Detect when panes were killed externally (e.g. Ctrl+Q/O).
		if m.splitPaneID != "" && !splitOpen() {
			unbindHostSplitKeys()
			m.splitPaneID = ""
			m.splitInstance = ""
			m.splitSession = ""
			m.activeGroupID = ""
		}
		return m, m.sessionPollLoop(true)

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
			return m, nil
		}
		m.splitPaneID = msg.paneID
		m.splitInstance = msg.instance
		m.splitSession = msg.session
		m.activeGroupID = msg.groupID
		// Rebind outer tmux split keys so new splits open inside
		// this instance (and group) instead of spawning a local shell.
		bindHostSplitKeys(msg.instance, msg.groupID)
		// Force an immediate session poll so the active session
		// indicator updates without waiting for the 1-second cycle.
		return m, m.sessionPollLoop(false)

	case sessionsMsg:
		if msg.err != nil {
			m.sessions[msg.instanceKey] = &sessionDiscovery{err: msg.err, fetchedAt: time.Now()}
		} else {
			m.sessions[msg.instanceKey] = &sessionDiscovery{sessions: msg.sessions, fetchedAt: time.Now()}
		}
		m.buildRows()
		return m, nil

	case sessionCreatedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to create session: %v", msg.err)
		} else {
			m.message = "Session created"
		}
		// Re-list sessions to refresh the UI
		if m.expandedInstances[msg.instanceKey] {
			parts := strings.SplitN(msg.instanceKey, "/", 2)
			if len(parts) == 2 {
				if f, ok := m.st.Fleets[parts[0]]; ok {
					if inst, err := f.GetInstance(parts[1]); err == nil {
						b := m.instanceBackend(inst)
						return m, listSessionsCmd(b, inst.WorkspaceDir, msg.instanceKey)
					}
				}
			}
		}
		return m, nil

	case sessionRenamedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to rename session: %v", msg.err)
		} else {
			m.message = fmt.Sprintf("Renamed session %s → %s", msg.oldName, msg.newName)
		}
		// Re-list sessions to refresh the UI
		if m.expandedInstances[msg.instanceKey] {
			parts := strings.SplitN(msg.instanceKey, "/", 2)
			if len(parts) == 2 {
				if f, ok := m.st.Fleets[parts[0]]; ok {
					if inst, err := f.GetInstance(parts[1]); err == nil {
						b := m.instanceBackend(inst)
						return m, listSessionsCmd(b, inst.WorkspaceDir, msg.instanceKey)
					}
				}
			}
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

	case codespaceMachinesFetchedMsg:
		m.codespaceFetchingMachines = false
		if msg.err != nil {
			return m, nil // silently ignore — gh may not be authed
		}
		m.codespaceMachines = msg.machines
		// If no machine configured yet and machines exist, select the first one
		if m.cfg != nil && m.cfg.CodespacesSettings.Machine == "" && len(m.codespaceMachines) > 0 {
			m.cfg.CodespacesSettings.Machine = m.codespaceMachines[0].Name
			_ = state.SaveConfig(m.cfg)
		}
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
						// Check for a dotfiles warning file
						warnPath := filepath.Join(state.FleetDir(), "logs", fleetName+"-"+instName+".warn")
						if warnData, err := os.ReadFile(warnPath); err == nil {
							_ = os.Remove(warnPath)
							firstLine := strings.SplitN(strings.TrimSpace(string(warnData)), "\n", 2)[0]
							m.message = fmt.Sprintf("Instance %s is running — %s (press l for details)", key, firstLine)
						} else {
							m.message = fmt.Sprintf("Instance %s is running (container: %s)",
								key, inst.ContainerID[:min(12, len(inst.ContainerID))])
						}
					case fleet.StatusFailed:
						delete(m.creating, key)
						if inst.Backend == fleet.BackendCodespaces && strings.HasPrefix(inst.Error, codespacesbackend.ErrPrefixAuthScope) {
							m.mode = viewCodespacesAuth
							m.dialogFleet = fleetName
							m.dialogInst = instName
							m.message = ""
						} else if inst.Backend == fleet.BackendCodespaces && strings.HasPrefix(inst.Error, codespacesbackend.ErrPrefixMachine) {
							m.mode = viewCodespacesMachine
							m.dialogFleet = fleetName
							m.dialogInst = instName
							m.message = ""
						} else if inst.Backend == fleet.BackendCodespaces && strings.HasPrefix(inst.Error, codespacesbackend.ErrPrefixLimit) {
							m.mode = viewCodespacesLimit
							m.dialogFleet = fleetName
							m.dialogInst = instName
							m.message = ""
						} else {
							m.message = fmt.Sprintf("Failed to create %s: %s", key, inst.Error)
						}
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
	case viewPortForward:
		return m.updatePortForward(msg)
	case viewCodespacesAuth:
		return m.updateCodespacesAuth(msg)
	case viewCodespacesLimit:
		return m.updateCodespacesLimit(msg)
	case viewCodespacesMachine:
		return m.updateCodespacesMachine(msg)
	case viewCreateSession:
		return m.updateCreateSession(msg)
	case viewRenameSession:
		return m.updateRenameSession(msg)
	default:
		return m.updateNormal(msg)
	}
}

func (m model) View() string {
	if m.quitting {
		killAllSplitPanes()
		m.portForwards.Shutdown()
		if m.inHostTmux {
			rebindHostSessionKeys()
			unbindHostSplitKeys()
			unbindHostCloseKeys()
		}
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
