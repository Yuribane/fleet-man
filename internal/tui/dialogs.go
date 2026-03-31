package tui

import (
	"fmt"
	"os"
	"path/filepath"
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
				// Instance-level delete
				f, ok := m.st.Fleets[m.dialogFleet]
				if ok {
					inst, err := f.GetInstance(m.dialogInst)
					if err == nil {
						_ = m.instanceBackend(inst).Down(inst.ContainerID)
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
				for _, inst := range f.Instances {
					_ = m.instanceBackend(inst).Down(inst.ContainerID)
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

var backendTypeOptions = []fleet.BackendType{
	fleet.BackendDevcontainer,
	fleet.BackendCoder,
}

func nextBackendType(current fleet.BackendType, direction int) fleet.BackendType {
	idx := 0
	for i, bt := range backendTypeOptions {
		if bt == current {
			idx = i
			break
		}
	}
	idx = (idx + direction + len(backendTypeOptions)) % len(backendTypeOptions)
	return backendTypeOptions[idx]
}

func backendTypeLabel(bt fleet.BackendType) string {
	switch bt {
	case fleet.BackendCoder:
		return "Coder"
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

			key := fleetName + "/" + name
			m.creating[key] = true
			m.buildRows()
			m.mode = viewNormal
			m.textInput.Blur()
			m.message = fmt.Sprintf("Creating %s (%s)...", key, backendTypeLabel(bt))

			return m, createInstanceCmd(fleetName, name, f.Remote, bt)

		case "left", "h":
			m.dialogBackend = nextBackendType(m.dialogBackend, -1)
			return m, nil

		case "right", "l":
			m.dialogBackend = nextBackendType(m.dialogBackend, 1)
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
