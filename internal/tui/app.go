package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

type viewMode int

const (
	viewNormal viewMode = iota
	viewConfirmDelete
	viewConfirmDeleteFleetWarn
	viewAddInstance
	viewAddFleet
)

// row represents a single navigable row in the TUI.
type row struct {
	fleetName     string
	instance      *fleet.Instance
	isFleetHeader bool
}

type model struct {
	rows      []row
	cursor    int
	collapsed map[string]bool

	st  *state.State
	err error

	mode        viewMode
	dialogFleet string
	dialogInst  string
	textInput   textinput.Model

	spinner  spinner.Model
	creating map[string]bool // "fleet/instance" keys currently being created

	stats map[string]*devcontainer.ContainerStats // containerID → stats

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

	m := model{
		collapsed: make(map[string]bool),
		creating:  make(map[string]bool),
		stats:     make(map[string]*devcontainer.ContainerStats),
		textInput: ti,
		spinner:   sp,
	}
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

	// Sort fleet names for stable ordering
	names := make([]string, 0, len(m.st.Fleets))
	for name := range m.st.Fleets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		f := m.st.Fleets[name]
		m.rows = append(m.rows, row{fleetName: name, isFleetHeader: true})
		if !m.collapsed[name] {
			for _, inst := range f.Instances {
				m.rows = append(m.rows, row{fleetName: name, instance: inst})
			}
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m *model) currentFleetName() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rows[m.cursor].fleetName
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

type statsMsg struct {
	stats map[string]*devcontainer.ContainerStats
}

func fetchStatsCmd(ids []string, delay bool) tea.Cmd {
	return func() tea.Msg {
		if delay {
			time.Sleep(3 * time.Second)
		}
		if len(ids) == 0 {
			return statsMsg{}
		}
		dc := devcontainer.NewClient()
		stats, _ := dc.Stats(ids)
		return statsMsg{stats: stats}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchStatsCmd(m.containerIDs(), false))
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
		return m, fetchStatsCmd(m.containerIDs(), true)

	case instanceCreatedMsg:
		key := msg.fleet + "/" + msg.instance
		delete(m.creating, key)
		m.reload()
		m.message = fmt.Sprintf("Instance %s is running (container: %s)", key, msg.containerID[:min(12, len(msg.containerID))])
		return m, nil
	case instanceCreateErrMsg:
		key := msg.fleet + "/" + msg.instance
		delete(m.creating, key)
		// Remove the failed instance from state
		st, _ := state.Load()
		if st != nil {
			if f, ok := st.Fleets[msg.fleet]; ok {
				_ = f.RemoveInstance(msg.instance)
				_ = state.Save(st)
			}
		}
		m.reload()
		m.message = fmt.Sprintf("Failed to create %s: %v", key, msg.err)
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

func (m model) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		case " ", "tab":
			// Toggle collapse on fleet headers
			if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].isFleetHeader {
				name := m.rows[m.cursor].fleetName
				m.collapsed[name] = !m.collapsed[name]
				m.buildRows()
			}

		case "r":
			m.reload()
			m.message = "Refreshed"

		case "d":
			if m.cursor < 0 || m.cursor >= len(m.rows) {
				break
			}
			r := m.rows[m.cursor]
			m.dialogFleet = r.fleetName
			if r.isFleetHeader {
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
			_, inst := m.selectedInstance()
			if inst == nil {
				// If on a fleet header, toggle collapse
				if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].isFleetHeader {
					name := m.rows[m.cursor].fleetName
					m.collapsed[name] = !m.collapsed[name]
					m.buildRows()
				}
				break
			}
			banner := renderGradient(nameToBanner(inst.Name))
			return m, tea.ExecProcess(
				execWithBanner(banner, "devcontainer", "exec", "--workspace-folder", inst.WorkspaceDir, "bash"),
				func(err error) tea.Msg { return execDoneMsg{err} },
			)

		case "o":
			_, inst := m.selectedInstance()
			if inst == nil {
				m.message = "Select an instance"
				break
			}
			err := openInTerminal([]string{"devcontainer", "exec", "--workspace-folder", inst.WorkspaceDir, "bash"})
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
				m.message = "Select an instance"
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

func (m model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			if m.dialogInst == "" {
				// Fleet-level delete — check if it has instances for double confirm
				if f, ok := m.st.Fleets[m.dialogFleet]; ok && len(f.Instances) > 0 {
					m.mode = viewConfirmDeleteFleetWarn
					return m, nil
				}
				// Empty fleet, just remove it
				delete(m.st.Fleets, m.dialogFleet)
				delete(m.collapsed, m.dialogFleet)
				_ = state.Save(m.st)
				m.buildRows()
				m.message = fmt.Sprintf("Removed fleet %s", m.dialogFleet)
			} else {
				// Instance-level delete
				f, ok := m.st.Fleets[m.dialogFleet]
				if ok {
					inst, err := f.GetInstance(m.dialogInst)
					if err == nil {
						dc := devcontainer.NewClient()
						_ = dc.Down(inst.ContainerID)
						if inst.WorkspaceDir != "" {
							_ = os.RemoveAll(inst.WorkspaceDir)
						}
						_ = f.RemoveInstance(inst.Name)
						_ = state.Save(m.st)
						m.buildRows()
						m.message = fmt.Sprintf("Removed %s/%s", m.dialogFleet, m.dialogInst)
					}
				}
			}
			m.mode = viewNormal

		case "n", "N", "esc", "ctrl+c":
			m.mode = viewNormal
			m.message = "Cancelled"
		}
	}
	return m, nil
}

func (m model) updateConfirmDeleteFleetWarn(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			f, ok := m.st.Fleets[m.dialogFleet]
			if ok {
				dc := devcontainer.NewClient()
				for _, inst := range f.Instances {
					_ = dc.Down(inst.ContainerID)
					if inst.WorkspaceDir != "" {
						_ = os.RemoveAll(inst.WorkspaceDir)
					}
				}
				delete(m.st.Fleets, m.dialogFleet)
				delete(m.collapsed, m.dialogFleet)
				_ = state.Save(m.st)
				m.buildRows()
				m.message = fmt.Sprintf("Removed fleet %s", m.dialogFleet)
			}
			m.mode = viewNormal

		case "n", "N", "esc", "ctrl+c":
			m.mode = viewNormal
			m.message = "Cancelled"
		}
	}
	return m, nil
}

func (m model) updateAddInstance(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.textInput.Value())
			if name == "" {
				m.message = "Name cannot be empty"
				m.mode = viewNormal
				return m, nil
			}

			fleetName := m.dialogFleet
			f, ok := m.st.Fleets[fleetName]
			if !ok {
				m.message = fmt.Sprintf("Fleet %s not found", fleetName)
				m.mode = viewNormal
				return m, nil
			}

			// Check duplicate
			if _, err := f.GetInstance(name); err == nil {
				m.message = fmt.Sprintf("Instance %s/%s already exists", fleetName, name)
				m.mode = viewNormal
				return m, nil
			}

			// Add instance immediately with "creating" status
			wsDir := filepath.Join(state.WorkspacesDir(), fleetName, name, fleetName)
			inst := &fleet.Instance{
				Name:         name,
				Config:       ".devcontainer/devcontainer.json",
				WorkspaceDir: wsDir,
				CreatedAt:    time.Now(),
				Status:       fleet.StatusCreating,
			}
			_ = f.AddInstance(inst)
			_ = state.Save(m.st)

			key := fleetName + "/" + name
			m.creating[key] = true
			m.buildRows()
			m.mode = viewNormal
			m.textInput.Blur()
			m.message = fmt.Sprintf("Creating %s...", key)

			return m, createInstanceCmd(fleetName, name, f.Remote)

		case "esc", "ctrl+c":
			m.mode = viewNormal
			m.textInput.Blur()
			m.message = "Cancelled"
			return m, nil
		}
	}

	// Pass keystrokes to text input
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) updateAddFleet(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			repoURL := strings.TrimSpace(m.textInput.Value())
			if repoURL == "" {
				m.message = "URL cannot be empty"
				m.mode = viewNormal
				return m, nil
			}
			fleetName := fleet.FleetNameFromRemote(repoURL)
			if fleetName == "" {
				m.message = "Could not derive fleet name from URL"
				m.mode = viewNormal
				return m, nil
			}
			m.st.GetOrCreateFleet(fleetName, repoURL)
			_ = state.Save(m.st)
			m.buildRows()
			m.mode = viewNormal
			m.textInput.Blur()
			m.message = fmt.Sprintf("Added fleet %s", fleetName)
			return m, nil

		case "esc", "ctrl+c":
			m.mode = viewNormal
			m.textInput.Blur()
			m.message = "Cancelled"
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

type execDoneMsg struct{ err error }

type instanceCreatedMsg struct {
	fleet       string
	instance    string
	containerID string
}

type instanceCreateErrMsg struct {
	fleet    string
	instance string
	err      error
}

func createInstanceCmd(fleetName, instanceName, remoteURL string) tea.Cmd {
	return func() tea.Msg {
		wsDir := filepath.Join(state.WorkspacesDir(), fleetName, instanceName, fleetName)

		if err := os.MkdirAll(filepath.Dir(wsDir), 0755); err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, fmt.Errorf("mkdir: %w", err)}
		}

		gitClone := exec.Command("git", "clone", remoteURL, wsDir)
		if out, err := gitClone.CombinedOutput(); err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, fmt.Errorf("git clone: %w\n%s", err, out)}
		}

		dc := devcontainer.NewClient()
		result, err := dc.Up(wsDir)
		if err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, err}
		}

		// Update state with real container info
		st, err := state.Load()
		if err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, err}
		}

		if f, ok := st.Fleets[fleetName]; ok {
			if inst, err := f.GetInstance(instanceName); err == nil {
				inst.ContainerID = result.ContainerID
				inst.Status = fleet.StatusRunning
			}
		}

		if err := state.Save(st); err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, err}
		}

		return instanceCreatedMsg{fleetName, instanceName, result.ContainerID}
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

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

	if len(m.rows) == 0 {
		listContent.WriteString(dimStyle.Render("  No instances. Press 'a' to create one, or use 'fleet up <name>'."))
		listContent.WriteString("\n")
	}

	for i, r := range m.rows {
		isSelected := i == m.cursor
		cursor := "  "
		if isSelected {
			cursor = cursorStyle.Render("> ")
		}

		if r.isFleetHeader {
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
		} else {
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

			if m.creating[key] {
				listContent.WriteString(fmt.Sprintf("%s    %s %s",
					cursor, paddedName, status,
				))
			} else {
				// Show CPU/memory stats
				statsStr := ""
				if s, ok := m.stats[inst.ContainerID]; ok {
					statsStr = dimStyle.Render(fmt.Sprintf("  %4.0f mcpu  %6.1f MB", s.CPUMillicores, s.MemoryMB))
				}
				listContent.WriteString(fmt.Sprintf("%s    %s %s%s",
					cursor, paddedName, status, statsStr,
				))
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
		dialog := fmt.Sprintf(
			"%s\n\n%s %s\n%s %s\n\n%s",
			dialogTitle.Render("New instance"),
			dialogLabel.Render("Fleet:"),
			fleetExpandedStyle.Render(m.dialogFleet),
			dialogLabel.Render("Name: "),
			m.textInput.View(),
			dialogHint.Render("[enter] Create  [esc] Cancel"),
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

	// Help — wrap keys into lines that fit terminal width
	helpKeys := []string{
		"j/k: navigate", "space: expand/collapse", "enter/e: exec",
		"o: open terminal", "n: new fleet", "a: add instance", "d: delete",
		"c: code", "l: logs", "r: refresh", "q: quit",
	}
	maxW := m.width
	if maxW <= 0 {
		maxW = 80
	}
	var helpLines []string
	var cur string
	for _, k := range helpKeys {
		entry := k
		if cur != "" {
			entry = "  " + k
		}
		if cur != "" && len(cur)+len(entry) > maxW {
			helpLines = append(helpLines, cur)
			cur = k
		} else {
			cur += entry
		}
	}
	if cur != "" {
		helpLines = append(helpLines, cur)
	}
	b.WriteString(helpStyle.Render(strings.Join(helpLines, "\n")))
	b.WriteString("\n")

	return b.String()
}

func renderStatus(s fleet.InstanceStatus) string {
	switch s {
	case fleet.StatusRunning:
		return statusRunningStyle.Render("running")
	case fleet.StatusStopped:
		return statusStoppedStyle.Render("stopped")
	case fleet.StatusCreating:
		return statusCreatingStyle.Render("creating")
	default:
		return dimStyle.Render(string(s))
	}
}

func renderGradient(text string) string {
	// Gradient from light cyan to deep blue
	type rgb struct{ r, g, b float64 }
	from := rgb{130, 220, 255}
	to := rgb{60, 80, 200}

	lines := strings.Split(text, "\n")
	// Find max line length for consistent gradient
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	if maxLen == 0 {
		return text
	}

	var out strings.Builder
	for i, line := range lines {
		if i > 0 {
			out.WriteString("\n")
		}
		for j, ch := range line {
			if ch == ' ' {
				out.WriteRune(ch)
				continue
			}
			t := float64(j) / float64(maxLen)
			r := int(from.r + (to.r-from.r)*t)
			g := int(from.g + (to.g-from.g)*t)
			b := int(from.b + (to.b-from.b)*t)
			color := fmt.Sprintf("#%02x%02x%02x", r, g, b)
			out.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(string(ch)))
		}
	}
	return out.String()
}

// Run starts the TUI.
func Run() error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
