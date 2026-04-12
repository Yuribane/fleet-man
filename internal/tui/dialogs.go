package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

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
				// Instance-level delete (async with transitional status)
				f, ok := m.st.Fleets[m.dialogFleet]
				if ok {
					inst, err := f.GetInstance(m.dialogInst)
					if err == nil {
						inst.Status = fleet.StatusDeleting
						_ = state.Save(m.st)
						m.buildRows()
						m.mode = viewNormal
						dc := m.instanceBackend(inst)
						return m, deleteInstanceCmd(dc, m.dialogFleet, m.dialogInst, inst.ContainerID, inst.WorkspaceDir, m.portForwards)
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
			if ok && len(f.Instances) > 0 {
				// Mark all instances as deleting
				for _, inst := range f.Instances {
					inst.Status = fleet.StatusDeleting
				}
				_ = state.Save(m.st)
				m.buildRows()
				m.mode = viewNormal
				return m, deleteFleetCmd(m.backends, m.dialogFleet, f.Instances, m.portForwards)
			} else if ok {
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

// updateConfirmDeleteSession handles the session deletion confirmation dialog.
func (m model) updateConfirmDeleteSession(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.mode = viewNormal
			instKey := m.dialogFleet + "/" + m.dialogInst
			f, ok := m.st.Fleets[m.dialogFleet]
			if !ok {
				break
			}
			inst, err := f.GetInstance(m.dialogInst)
			if err != nil {
				break
			}
			b := m.instanceBackend(inst)
			sanitized := SanitizeSessionName(inst.Name)
			// If this is a grouped session, kill the entire group.
			if m.dialogGroupID != "" && isGroupedSession(sanitized, m.dialogSession) {
				return m, deleteGroupSessionsCmd(b, inst.WorkspaceDir, instKey, sanitized, m.dialogGroupID)
			}
			return m, deleteSessionCmd(b, inst.WorkspaceDir, instKey, m.dialogSession)

		case "n", "N", "esc", "ctrl+c":
			m.mode = viewNormal
			m.message = "Cancelled"
		}
	}
	return m, nil
}

// backendToolRequirements maps each backend type to the CLI binary it
// requires.  An empty string means no external tool is needed.
var backendToolRequirements = map[fleet.BackendType]string{
	fleet.BackendDevcontainer: "devcontainer",
	fleet.BackendCoder:        "coder",
	fleet.BackendCodespaces:   "gh",
}

// allBackendTypes is the ordered master list of every backend type.
var allBackendTypes = []fleet.BackendType{
	fleet.BackendDevcontainer,
	fleet.BackendCoder,
	fleet.BackendCodespaces,
}

// availableBackendTypes returns the subset of backend types whose
// required CLI tool is found on the system.
func (m model) availableBackendTypes() []fleet.BackendType {
	var out []fleet.BackendType
	for _, bt := range allBackendTypes {
		bin := backendToolRequirements[bt]
		if bin == "" {
			out = append(out, bt)
			continue
		}
		for _, t := range m.toolStatus {
			if t.Binary == bin && t.Found {
				out = append(out, bt)
				break
			}
		}
	}
	return out
}

// nextBackendType cycles through the given options list from current.
func nextBackendType(current fleet.BackendType, direction int, options []fleet.BackendType) fleet.BackendType {
	if len(options) == 0 {
		return current
	}
	idx := 0
	for i, bt := range options {
		if bt == current {
			idx = i
			break
		}
	}
	idx = (idx + direction + len(options)) % len(options)
	return options[idx]
}

func backendTypeLabel(bt fleet.BackendType) string {
	switch bt {
	case fleet.BackendCoder:
		return "Coder"
	case fleet.BackendCodespaces:
		return "Codespaces"
	default:
		return "Devcontainer"
	}
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

			bt := m.dialogBackend
			if bt == "" {
				bt = fleet.BackendDevcontainer
			}

			// Add instance immediately with "creating" status
			wsDir := filepath.Join(state.WorkspacesDir(), fleetName, name, fleetName)
			inst := &fleet.Instance{
				Name:         name,
				Config:       ".devcontainer/devcontainer.json",
				WorkspaceDir: wsDir,
				CreatedAt:    time.Now(),
				Status:       fleet.StatusCreating,
				Backend:      bt,
			}
			_ = f.AddInstance(inst)
			_ = state.Save(m.st)

			// Remember the backend choice for next time
			if m.cfg != nil {
				m.cfg.DefaultBackend = string(bt)
				_ = state.SaveConfig(m.cfg)
			}

			key := fleetName + "/" + name
			m.creating[key] = true
			m.buildRows()
			m.mode = viewNormal
			m.textInput.Blur()
			m.message = fmt.Sprintf("Creating %s (%s)...", key, backendTypeLabel(bt))

			return m, createInstanceCmd(fleetName, name, f.Remote, bt)

		case "tab":
			opts := m.availableBackendTypes()
			if len(opts) > 1 {
				m.dialogBackend = nextBackendType(m.dialogBackend, 1, opts)
			}
			return m, nil

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

func (m model) updateTagInstance(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			tag := strings.TrimSpace(m.textInput.Value())

			f, ok := m.st.Fleets[m.dialogFleet]
			if ok {
				if inst, err := f.GetInstance(m.dialogInst); err == nil {
					inst.Tag = tag
					_ = state.Save(m.st)
				}
			}

			m.mode = viewNormal
			m.textInput.Blur()
			if tag == "" {
				m.message = fmt.Sprintf("Cleared tag for %s/%s", m.dialogFleet, m.dialogInst)
			} else {
				m.message = fmt.Sprintf("Tagged %s/%s: %s", m.dialogFleet, m.dialogInst, tag)
			}
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

// ===========================================
// Port Forward Dialog
// ===========================================

// updatePortForward handles the port forward management dialog.
// The user can type a "local:remote" mapping and press enter to add,
// use up/down to select existing forwards, 'd' to delete one, or esc to close.
func (m model) updatePortForward(msg tea.Msg) (tea.Model, tea.Cmd) {
	key := m.dialogFleet + "/" + m.dialogInst

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			raw := strings.TrimSpace(m.textInput.Value())
			if raw == "" {
				return m, nil
			}
			local, remote, err := parsePortMapping(raw)
			if err != nil {
				m.message = err.Error()
				return m, nil
			}

			b := m.instanceBackend(&fleet.Instance{Backend: m.instanceBackendType()})
			if err := m.portForwards.Add(key, local, remote, b.PortForwardCommand, m.pfContainerID, b.ResolveHostname); err != nil {
				m.message = err.Error()
				return m, nil
			}

			m.textInput.SetValue("")
			m.message = fmt.Sprintf("Forwarding localhost:%d -> %s:%d", local, m.dialogInst, remote)
			return m, nil

		case "d":
			fwds := m.portForwards.List(key)
			if len(fwds) > 0 && m.pfCursor < len(fwds) {
				fwd := fwds[m.pfCursor]
				_ = m.portForwards.Remove(key, fwd.LocalPort)
				m.message = fmt.Sprintf("Removed forward %s", fwd.Label())
				if m.pfCursor >= len(m.portForwards.List(key)) {
					m.pfCursor = max(0, m.pfCursor-1)
				}
				return m, nil
			}

		case "up", "k":
			if m.pfCursor > 0 {
				m.pfCursor--
			}
			return m, nil

		case "down", "j":
			fwds := m.portForwards.List(key)
			if m.pfCursor < len(fwds)-1 {
				m.pfCursor++
			}
			return m, nil

		case "esc", "ctrl+c":
			m.mode = viewNormal
			m.textInput.Blur()
			fwds := m.portForwards.List(key)
			if len(fwds) > 0 {
				m.message = fmt.Sprintf("%d port forward(s) active on %s", len(fwds), m.dialogInst)
			} else {
				m.message = ""
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// parsePortMapping splits a "local:remote" string into two port numbers.
func parsePortMapping(raw string) (int, int, error) {
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected local:remote (e.g. 8080:80)")
	}
	local, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || local < 1 || local > 65535 {
		return 0, 0, fmt.Errorf("invalid local port %q", parts[0])
	}
	remote, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || remote < 1 || remote > 65535 {
		return 0, 0, fmt.Errorf("invalid remote port %q", parts[1])
	}
	return local, remote, nil
}

// ===========================================
// Codespaces Auth Scope Dialog
// ===========================================

// updateCodespacesAuth handles the dialog shown when gh is missing
// the "codespace" OAuth scope. Enter launches the auth refresh flow.
func (m model) updateCodespacesAuth(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.mode = viewNormal
			m.message = "Launching GitHub auth..."
			return m, tea.ExecProcess(
				exec.Command("gh", "auth", "login", "-h", "github.com", "-s", "codespace"),
				func(err error) tea.Msg {
					if err != nil {
						return execDoneMsg{fmt.Errorf("gh auth login: %w", err)}
					}
					return execDoneMsg{}
				},
			)
		case "esc", "ctrl+c":
			m.mode = viewNormal
			m.message = "Auth cancelled — codespace creation requires the codespace scope"
		}
	}
	return m, nil
}

// updateCodespacesMachine handles the dialog shown when gh needs a
// machine type but none is configured. Enter navigates to settings.
func (m model) updateCodespacesMachine(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.mode = viewNormal
			m.page = pageSettings
			m.message = "Set the Machine field, then retry instance creation"
			return m, nil
		case "esc", "ctrl+c":
			m.mode = viewNormal
			m.message = ""
		}
	}
	return m, nil
}

// updateCodespacesLimit handles the dialog shown when the user has
// hit the maximum codespace count.
func (m model) updateCodespacesLimit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc", "ctrl+c":
			m.mode = viewNormal
			m.message = ""
		}
	}
	return m, nil
}

// ===========================================
// Port Forward Helpers
// ===========================================

// instanceBackendType returns the backend type for the instance currently
// being managed in the port forward dialog.
func (m model) instanceBackendType() fleet.BackendType {
	if f, ok := m.st.Fleets[m.dialogFleet]; ok {
		if inst, err := f.GetInstance(m.dialogInst); err == nil {
			return inst.Backend
		}
	}
	return fleet.BackendDevcontainer
}

// ===========================================
// Session Dialogs
// ===========================================

// updateCreateSession handles the dialog for creating a new tmux session
// inside a running instance. An empty name auto-generates one.
func (m model) updateCreateSession(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.textInput.Value())
			instKey := m.dialogFleet + "/" + m.dialogInst
			if name == "" {
				var existing []tmuxSession
				if disc, ok := m.sessions[instKey]; ok && disc.err == nil {
					existing = disc.sessions
				}
				name = nextSessionName(existing)
			}
			name = SanitizeSessionName(name)

			f, ok := m.st.Fleets[m.dialogFleet]
			if !ok {
				m.mode = viewNormal
				m.textInput.Blur()
				return m, nil
			}
			inst, err := f.GetInstance(m.dialogInst)
			if err != nil {
				m.mode = viewNormal
				m.textInput.Blur()
				return m, nil
			}

			// Apply group naming convention so the session is
			// recognized as a group root (instance~name).
			sanitized := SanitizeSessionName(inst.Name)
			fullName := groupSessionName(sanitized, name)

			m.mode = viewNormal
			m.textInput.Blur()
			m.message = fmt.Sprintf("Creating session %s...", name)
			b := m.instanceBackend(inst)
			return m, createSessionCmd(b, inst.WorkspaceDir, instKey, fullName)

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

// updateRenameSession handles the dialog for renaming a tmux session.
func (m model) updateRenameSession(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			newName := strings.TrimSpace(m.textInput.Value())
			if newName == "" {
				m.message = "Name cannot be empty"
				m.mode = viewNormal
				m.textInput.Blur()
				return m, nil
			}
			newName = SanitizeSessionName(newName)

			instKey := m.dialogFleet + "/" + m.dialogInst

			f, ok := m.st.Fleets[m.dialogFleet]
			if !ok {
				m.mode = viewNormal
				m.textInput.Blur()
				return m, nil
			}
			inst, err := f.GetInstance(m.dialogInst)
			if err != nil {
				m.mode = viewNormal
				m.textInput.Blur()
				return m, nil
			}

			sanitized := SanitizeSessionName(inst.Name)
			oldName := m.dialogSession
			oldGID, isGrouped := parseGroupID(sanitized, oldName)

			m.mode = viewNormal
			m.textInput.Blur()
			b := m.instanceBackend(inst)

			if isGrouped {
				// Rename the entire group: swap old group ID for new
				// in all sessions matching the prefix.
				return m, renameGroupCmd(b, inst.WorkspaceDir, instKey, sanitized, oldGID, newName)
			}
			// Ungrouped/legacy session — rename just the one.
			return m, renameSessionCmd(b, inst.WorkspaceDir, instKey, oldName, newName)

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
