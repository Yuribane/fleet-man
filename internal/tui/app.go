package tui

import (
	"context"
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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ===========================================
// Model
// ===========================================

type model struct {
	st  *state.State
	cfg *state.Config
	err error

	// Page routing
	currentPage Page
	fleetPage   *fleetPage // persistent — has running state accessed by background message handlers

	spinner  spinner.Model
	creating map[string]bool // "fleet/instance" keys currently being created

	backends map[fleet.BackendType]backend.Backend // one per backend type, lazily created
	stats    map[string]*backend.ContainerStats    // containerID → stats
	activity *ActivityTracker                      // agent working/waiting/idle detection

	coderPresets        []string // available preset names (in-memory, from API)
	coderFetchingParams bool     // true while fetching template parameters

	codespaceMachines         []codespaceMachine // available machine types (from GitHub API)
	codespaceFetchingMachines bool               // true while fetching machine types

	toolStatus []deps.ToolStatus // cached tool install statuses for settings page

	// Port forwarding
	portForwards *portforward.Manager // manages active port forward processes

	// Instance expansion: which instances are expanded to show sessions
	expandedInstances map[string]bool              // key: "fleet/instance"
	sessions          map[string]*sessionDiscovery // key: "fleet/instance"

	// Last active session per instance (in-memory only). Used to
	// reconnect to the previous session when pressing enter on an
	// instance row instead of always creating a new one.
	lastActive map[string]lastSession // key: "fleet/instance"

	// Split pane mode: when fleet runs inside a host tmux session,
	// pressing enter opens the instance shell in a right-side pane
	// instead of suspending the TUI.
	inHostTmux bool // true when TMUX env var is set at startup

	// Update check
	updateAvailable string // non-empty = new version tag from GitHub

	message  string
	quitting bool
	width    int
	height   int
}

// newModel creates and initialises the top-level model, including all
// page instances and their initial state.
func newModel() model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))

	m := model{
		creating:          make(map[string]bool),
		backends:          make(map[fleet.BackendType]backend.Backend),
		stats:             make(map[string]*backend.ContainerStats),
		activity:          NewActivityTracker(),
		portForwards:      portforward.NewManager(),
		expandedInstances: make(map[string]bool),
		sessions:          make(map[string]*sessionDiscovery),
		lastActive:        make(map[string]lastSession),
		spinner:           sp,
		inHostTmux:        os.Getenv("TMUX") != "",
	}

	// Create the fleet page (persistent — background handlers reference it)
	m.fleetPage = newFleetPage()
	m.currentPage = m.fleetPage

	// Unbind C-PPage/C-NPage from the host tmux so they pass through
	// to inner tmux sessions for session cycling. Bind Ctrl+Q/O to
	// close all split panes from any pane.
	if m.inHostTmux {
		bindHostSessionCycleKeys()
		bindHostCloseKeys()
		// Neutralise default split bindings so the user doesn't
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
			m.currentPage = newDepsCheckPage(result)
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

// ===========================================
// State Management
// ===========================================

// reload refreshes state and config from disk and prunes stale
// expanded instances. It does NOT rebuild rows — the active page
// is responsible for that.
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
}

// ===========================================
// Backend Helpers
// ===========================================

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

// ===========================================
// Session Discovery
// ===========================================

// sessionDiscoveryLoop returns a tea.Cmd that lists tmux sessions for
// expanded instances on a 1-second cycle.
func (m model) sessionDiscoveryLoop() tea.Cmd {
	return sessionDiscoveryCmd(m.backends, m.expandedInstances, m.st.Fleets)
}

// refreshInstanceSessions returns a tea.Cmd that re-lists tmux sessions
// for the given instance (if expanded). Used after split pane creation,
// group switching, and session creation to keep the UI in sync.
func (m *model) refreshInstanceSessions(instanceName string) tea.Cmd {
	for instKey, expanded := range m.expandedInstances {
		if !expanded {
			continue
		}
		parts := strings.SplitN(instKey, "/", 2)
		if len(parts) != 2 || parts[1] != instanceName {
			continue
		}
		if f, ok := m.st.Fleets[parts[0]]; ok {
			if inst, err := f.GetInstance(parts[1]); err == nil {
				b := m.instanceBackend(inst)
				return listSessionsCmd(b, inst.WorkspaceDir, instKey)
			}
		}
	}
	return nil
}

// ===========================================
// Stats
// ===========================================

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

// ===========================================
// Bubbletea Lifecycle
// ===========================================

// Init returns the initial set of commands for the program.
func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.fetchAllStatsCmd(false),
		m.sessionDiscoveryLoop(),
		checkUpdateCmd(),
		forceRepaintCmd(),
		m.currentPage.Init(&m),
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

// Update handles a single Bubbletea message. Shared-only messages are
// handled here and returned early. Mixed messages handle their shared
// part then fall through. Everything else is forwarded to the active page.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Window size (shared)
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
	}

	// 2. Always update spinner
	var spinCmd tea.Cmd
	m.spinner, spinCmd = m.spinner.Update(msg)

	// 3. Shared-only messages — return early
	switch msg := msg.(type) {
	case statsMsg:
		if msg.stats != nil {
			m.stats = msg.stats
		}
		if msg.screens != nil {
			m.activity.Update(msg.screens, msg.probes, msg.containerIDs, time.Now())
		}
		return m, tea.Batch(spinCmd, m.fetchAllStatsCmd(true))

	case updateCheckMsg:
		if msg.latestVersion != "" {
			m.updateAvailable = msg.latestVersion
		}
		return m, spinCmd

	case coderParamsFetchedMsg:
		m.coderFetchingParams = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to fetch parameters: %v", msg.err)
			return m, spinCmd
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
			val := existing[rp.Name]
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
		m.coderPresets = nil
		for _, p := range msg.presets {
			m.coderPresets = append(m.coderPresets, p.Name)
		}
		if m.cfg.CoderSettings.Preset == "" && len(m.coderPresets) > 0 {
			m.cfg.CoderSettings.Preset = m.coderPresets[0]
		}
		_ = state.SaveConfig(m.cfg)
		m.message = fmt.Sprintf("Loaded %d parameters, %d presets", len(newParams), len(m.coderPresets))
		return m, spinCmd

	case codespaceMachinesFetchedMsg:
		m.codespaceFetchingMachines = false
		if msg.err != nil {
			return m, spinCmd
		}
		m.codespaceMachines = msg.machines
		if m.cfg != nil && m.cfg.CodespacesSettings.Machine == "" && len(m.codespaceMachines) > 0 {
			m.cfg.CodespacesSettings.Machine = m.codespaceMachines[0].Name
			_ = state.SaveConfig(m.cfg)
		}
		return m, spinCmd

	case forceRepaintTickMsg:
		// Scrub artifacts left by outer-tmux pane resizes without
		// flicker. A synthetic WindowSizeMsg (with the current
		// dimensions so no app-level resize happens) causes
		// bubbletea's renderer to invalidate its per-line cache; on
		// the next flush every tracked line is rewritten with
		// EraseLineRight appended, scrubbing stale chars inside the
		// TUI's bounds. The trailing EraseScreenBelow escape that
		// View() tacks onto the last line then clears everything
		// beneath the TUI — and because that escape is part of the
		// view string, the clear lands in the same atomic buffer
		// flush as the redraw, so the terminal never sees a blank
		// frame (unlike tea.ClearScreen which writes the erase
		// ahead of the next render tick).
		return m, tea.Batch(
			spinCmd,
			func() tea.Msg { return tea.WindowSizeMsg{Width: m.width, Height: m.height} },
			forceRepaintCmd(),
		)
	}

	// 4. Mixed messages — handle shared part, then forward to page
	var extraCmds []tea.Cmd
	switch msg := msg.(type) {
	case sessionDiscoveryMsg:
		if msg.discovered != nil {
			for key, sessions := range msg.discovered {
				m.sessions[key] = &sessionDiscovery{sessions: sessions, fetchedAt: time.Now()}
			}
			// Clear lastActive entries that point to sessions/groups
			// that no longer exist.
			for key, last := range m.lastActive {
				disc, ok := m.sessions[key]
				if !ok || disc.err != nil {
					delete(m.lastActive, key)
					continue
				}
				if !sessionStillExists(last, disc.sessions) {
					delete(m.lastActive, key)
				}
			}
		}
		extraCmds = append(extraCmds, m.sessionDiscoveryLoop())

	case operationDoneMsg:
		m.reload()
		if msg.err != nil {
			m.message = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.message = msg.message
		}

	case instanceCreateErrMsg:
		key := msg.fleet + "/" + msg.instance
		delete(m.creating, key)
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

	case splitPaneMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Split pane error: %v", msg.err)
		} else {
			fp := m.fleetPage
			fp.splitPaneID = msg.paneID
			fp.splitFleet = msg.fleet
			fp.splitInstance = msg.instance
			fp.splitSession = msg.session
			fp.activeGroupID = msg.groupID
			instKey := msg.fleet + "/" + msg.instance
			m.lastActive[instKey] = lastSession{sessionName: msg.session, groupID: msg.groupID}
			bindHostSplitKeys(instKey, msg.groupID)
			extraCmds = append(extraCmds, m.refreshInstanceSessions(msg.instance))
		}

	case sessionsMsg:
		if msg.err != nil {
			m.sessions[msg.instanceKey] = &sessionDiscovery{err: msg.err, fetchedAt: time.Now()}
		} else {
			m.sessions[msg.instanceKey] = &sessionDiscovery{sessions: msg.sessions, fetchedAt: time.Now()}
		}

	case sessionCreatedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to create session: %v", msg.err)
		} else {
			m.message = "Session created"
		}
		if m.expandedInstances[msg.instanceKey] {
			parts := strings.SplitN(msg.instanceKey, "/", 2)
			if len(parts) == 2 {
				if f, ok := m.st.Fleets[parts[0]]; ok {
					if inst, err := f.GetInstance(parts[1]); err == nil {
						b := m.instanceBackend(inst)
						extraCmds = append(extraCmds, listSessionsCmd(b, inst.WorkspaceDir, msg.instanceKey))
					}
				}
			}
		}

	case sessionRenamedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to rename session: %v", msg.err)
		} else {
			m.message = fmt.Sprintf("Renamed session %s → %s", msg.oldName, msg.newName)
		}
		if m.expandedInstances[msg.instanceKey] {
			parts := strings.SplitN(msg.instanceKey, "/", 2)
			if len(parts) == 2 {
				if f, ok := m.st.Fleets[parts[0]]; ok {
					if inst, err := f.GetInstance(parts[1]); err == nil {
						b := m.instanceBackend(inst)
						extraCmds = append(extraCmds, listSessionsCmd(b, inst.WorkspaceDir, msg.instanceKey))
					}
				}
			}
		}

	case sessionDeletedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed to delete session: %v", msg.err)
		} else {
			m.message = fmt.Sprintf("Deleted session %s", msg.sessionName)
		}
		if last, ok := m.lastActive[msg.instanceKey]; ok {
			if last.sessionName == msg.sessionName || last.groupID == msg.groupID {
				delete(m.lastActive, msg.instanceKey)
			}
		}
		fp := m.fleetPage
		if fp.splitSession == msg.sessionName || (msg.groupID != "" && fp.activeGroupID == msg.groupID) {
			if fp.splitPaneID != "" {
				killAllSplitPanes()
				unbindHostSplitKeys()
			}
			fp.splitPaneID = ""
			fp.splitFleet = ""
			fp.splitInstance = ""
			fp.splitSession = ""
			fp.activeGroupID = ""
		}
		if m.expandedInstances[msg.instanceKey] {
			parts := strings.SplitN(msg.instanceKey, "/", 2)
			if len(parts) == 2 {
				if f, ok := m.st.Fleets[parts[0]]; ok {
					if inst, err := f.GetInstance(parts[1]); err == nil {
						b := m.instanceBackend(inst)
						extraCmds = append(extraCmds, listSessionsCmd(b, inst.WorkspaceDir, msg.instanceKey))
					}
				}
			}
		}

	case pollCreatingTickMsg:
		if len(m.creating) > 0 {
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
							fp := m.fleetPage
							if inst.Backend == fleet.BackendCodespaces && strings.HasPrefix(inst.Error, codespacesbackend.ErrPrefixAuthScope) {
								fp.mode = viewCodespacesAuth
								fp.dialogFleet = fleetName
								fp.dialogInst = instName
								m.message = ""
							} else if inst.Backend == fleet.BackendCodespaces && strings.HasPrefix(inst.Error, codespacesbackend.ErrPrefixMachine) {
								fp.mode = viewCodespacesMachine
								fp.dialogFleet = fleetName
								fp.dialogInst = instName
								m.message = ""
							} else if inst.Backend == fleet.BackendCodespaces && strings.HasPrefix(inst.Error, codespacesbackend.ErrPrefixLimit) {
								fp.mode = viewCodespacesLimit
								fp.dialogFleet = fleetName
								fp.dialogInst = instName
								m.message = ""
							} else {
								m.message = fmt.Sprintf("Failed to create %s: %s", key, inst.Error)
							}
						}
					}
				}
			}
			if len(m.creating) > 0 {
				extraCmds = append(extraCmds, pollCreatingCmd())
			}
		}

	case instanceSpawnedMsg:
		extraCmds = append(extraCmds, pollCreatingCmd())

	case groupCycleMsg:
		fp := m.fleetPage
		if msg.seq == fp.debounceSeq && fp.pendingGroupID != "" {
			cmd := fp.commitGroupCycle(&m)
			extraCmds = append(extraCmds, cmd)
		}
	}

	// 5. Forward to current page
	pageCmd := m.currentPage.Update(&m, msg)

	// 6. Return
	allCmds := []tea.Cmd{spinCmd, pageCmd}
	allCmds = append(allCmds, extraCmds...)
	return m, tea.Batch(allCmds...)
}

// View renders the current page. On quit it cleans up split panes
// and port forwards.
func (m model) View() string {
	if m.quitting {
		// Clean up split panes via the fleet page
		fp := m.fleetPage
		if fp.splitPaneID != "" {
			killAllSplitPanes()
		}
		m.portForwards.Shutdown()
		if m.inHostTmux {
			unbindHostSessionCycleKeys()
			unbindHostSplitKeys()
			unbindHostCloseKeys()
		}
		return ""
	}
	// Append EraseScreenBelow (\x1b[0J) so that whenever bubbletea
	// rewrites the last line, it also scrubs any stale characters
	// sitting beneath the TUI. This is a no-op while the last line's
	// content is unchanged (bubbletea's line-diff skips the line, the
	// escape is not re-executed). On the 1-second repaint tick the
	// accompanying WindowSizeMsg invalidates the full line cache,
	// forcing the last line — and thus this escape — to be rewritten
	// inside the same atomic buffer flush as the rest of the redraw.
	return m.currentPage.View(&m) + "\x1b[0J"
}

// ===========================================
// Row Types (shared, used by fleet page)
// ===========================================

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

// lastSession tracks the most recently used session for an instance,
// allowing reconnection on subsequent enter presses instead of always
// creating a new session.
type lastSession struct {
	sessionName string
	groupID     string
}

// ===========================================
// Sorting Helper
// ===========================================

// sortedFleetNames returns fleet names in stable alphabetical order.
func sortedFleetNames(fleets map[string]*fleet.Fleet) []string {
	names := make([]string, 0, len(fleets))
	for name := range fleets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ===========================================
// Entry Point
// ===========================================

// Run starts the TUI.
func Run() error {
	m := newModel()

	// Start clipboard buffer polling when running inside tmux.
	// A goroutine polls `tmux show-buffer` and copies changes to the
	// system clipboard (wl-copy / xclip / pbcopy). This is the
	// universal clipboard mechanism that works on ALL terminals,
	// including those without OSC 52 support.
	var clipCancel context.CancelFunc
	if m.inHostTmux {
		if cs := newClipboardSync(); cs != nil {
			ctx, cancel := context.WithCancel(context.Background())
			clipCancel = cancel
			go cs.Start(ctx)
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()

	if clipCancel != nil {
		clipCancel()
	}
	return err
}
