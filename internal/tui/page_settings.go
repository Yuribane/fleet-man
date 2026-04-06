package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/doctor"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	settingsItemToolSelection = iota
	settingsItemDotfilesRepo
	settingsItemDotfilesScript
	settingsItemDotfilesAutoInstall
	settingsItemCoderTemplate
	settingsItemCoderPreset
	settingsItemCoderParamBase // parameters are at index base + i

	settingsItemCodespacesMachine = 500 // codespaces settings start here
	settingsItemCodespacesIdle    = 501
	settingsItemCodespacesDevcontainer = 502

	settingsItemToolStatusBase = 1000 // tool status rows start here
	settingsItemDoctor         = 2000 // doctor action row
)

// settingsSection defines a titled group of settings rows that can be
// conditionally shown based on tool availability.
type settingsSection struct {
	Title string                          // section header text
	Tool  string                          // required tool binary; "" = always visible
	Items func(cfg *state.Config) []int   // returns navigable item IDs for this section
}

// settingsSections lists all settings sections in display order.
var settingsSections = []settingsSection{
	{
		Title: "Agent Settings",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemToolSelection}
		},
	},
	{
		Title: "Dotfiles",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemDotfilesRepo, settingsItemDotfilesScript, settingsItemDotfilesAutoInstall}
		},
	},
	{
		Title: "Coder",
		Tool:  "coder",
		Items: func(cfg *state.Config) []int {
			items := []int{settingsItemCoderTemplate, settingsItemCoderPreset}
			if cfg != nil {
				for i := range cfg.CoderSettings.Parameters {
					items = append(items, settingsItemCoderParamBase+i)
				}
			}
			return items
		},
	},
	{
		Title: "Codespaces",
		Tool:  "gh",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemCodespacesMachine, settingsItemCodespacesIdle, settingsItemCodespacesDevcontainer}
		},
	},
	{
		Title: "Tool Status",
		Items: func(_ *state.Config) []int {
			return []int{
				settingsItemToolStatusBase,
				settingsItemToolStatusBase + 1,
				settingsItemToolStatusBase + 2,
			}
		},
	},
	{
		Title: "Doctor",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemDoctor}
		},
	},
}

// sectionVisible reports whether a settings section should be shown
// based on the current tool install status.
func (m model) sectionVisible(s settingsSection) bool {
	if s.Tool == "" {
		return true
	}
	for _, t := range m.toolStatus {
		if t.Binary == s.Tool {
			return t.Found
		}
	}
	return false
}

// visibleItems returns the flat ordered list of navigable item IDs,
// skipping items in hidden sections.
func (m model) visibleItems() []int {
	var items []int
	for _, s := range settingsSections {
		if !m.sectionVisible(s) {
			continue
		}
		items = append(items, s.Items(m.cfg)...)
	}
	return items
}

// settingsCursorItem returns the item ID at the current cursor position,
// or -1 if the cursor is out of range.
func (m model) settingsCursorItem() int {
	items := m.visibleItems()
	if m.settingsCursor >= 0 && m.settingsCursor < len(items) {
		return items[m.settingsCursor]
	}
	return -1
}

// settingsItemCount returns the total number of navigable settings rows.
func (m model) settingsItemCount() int {
	return len(m.visibleItems())
}

var agentToolOptions = []state.AgentTool{
	state.AgentToolCodex,
	state.AgentToolClaude,
	state.AgentToolGemini,
	state.AgentToolCopilot,
}

func nextAgentTool(current state.AgentTool, direction int) state.AgentTool {
	idx := 0
	for i, tool := range agentToolOptions {
		if tool == current {
			idx = i
			break
		}
	}

	idx = (idx + direction + len(agentToolOptions)) % len(agentToolOptions)
	return agentToolOptions[idx]
}

func agentToolLabel(tool state.AgentTool) string {
	switch tool {
	case state.AgentToolCodex:
		return "Codex"
	case state.AgentToolClaude:
		return "Claude Code"
	case state.AgentToolGemini:
		return "Gemini"
	case state.AgentToolCopilot:
		return "Copilot"
	default:
		return "Claude Code"
	}
}

func (m *model) cycleAgentTool(direction int) {
	if m.cfg == nil {
		m.cfg = state.DefaultConfig()
	}

	current := m.cfg.AgentSettings.ToolSelection
	next := nextAgentTool(current, direction)
	if next == current {
		return
	}

	m.cfg.AgentSettings.ToolSelection = next
	if err := state.SaveConfig(m.cfg); err != nil {
		m.cfg.AgentSettings.ToolSelection = current
		m.message = fmt.Sprintf("Failed to save settings: %v", err)
		return
	}

	m.message = fmt.Sprintf("Preferred tool set to %s", agentToolLabel(next))
}

func (m *model) toggleAutoInstall() {
	if m.cfg == nil {
		m.cfg = state.DefaultConfig()
	}

	m.cfg.DotfilesSettings.AutoInstall = !m.cfg.DotfilesSettings.AutoInstall
	if err := state.SaveConfig(m.cfg); err != nil {
		m.cfg.DotfilesSettings.AutoInstall = !m.cfg.DotfilesSettings.AutoInstall
		m.message = fmt.Sprintf("Failed to save settings: %v", err)
		return
	}

	label := "off"
	if m.cfg.DotfilesSettings.AutoInstall {
		label = "on"
	}
	m.message = fmt.Sprintf("Auto install dotfiles set to %s", label)
}

func (m *model) cycleCoderPreset(direction int) {
	if m.cfg == nil || len(m.coderPresets) == 0 {
		return
	}

	current := m.cfg.CoderSettings.Preset
	idx := 0
	for i, p := range m.coderPresets {
		if p == current {
			idx = i
			break
		}
	}

	idx = (idx + direction + len(m.coderPresets)) % len(m.coderPresets)
	m.cfg.CoderSettings.Preset = m.coderPresets[idx]
	if err := state.SaveConfig(m.cfg); err != nil {
		m.cfg.CoderSettings.Preset = current
		m.message = fmt.Sprintf("Failed to save settings: %v", err)
		return
	}

	m.message = fmt.Sprintf("Preset set to %s", m.cfg.CoderSettings.Preset)
}

func (m *model) cycleCodespacesMachine(direction int) {
	if m.cfg == nil || len(m.codespaceMachines) == 0 {
		return
	}

	current := m.cfg.CodespacesSettings.Machine
	idx := 0
	for i, mt := range m.codespaceMachines {
		if mt.Name == current {
			idx = i
			break
		}
	}

	idx = (idx + direction + len(m.codespaceMachines)) % len(m.codespaceMachines)
	selected := m.codespaceMachines[idx]
	m.cfg.CodespacesSettings.Machine = selected.Name
	if err := state.SaveConfig(m.cfg); err != nil {
		m.cfg.CodespacesSettings.Machine = current
		m.message = fmt.Sprintf("Failed to save settings: %v", err)
		return
	}

	m.message = fmt.Sprintf("Machine set to %s", selected.DisplayName)
}

// codespacesMachineLabel returns the display label for the currently
// configured machine. Falls back to the raw name if no match is found.
func (m *model) codespacesMachineLabel() string {
	name := m.cfg.CodespacesSettings.Machine
	for _, mt := range m.codespaceMachines {
		if mt.Name == name {
			return mt.DisplayName
		}
	}
	return name
}

func (m model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.settingsEditing {
		return m.updateSettingsEditing(msg)
	}
	return m.updateSettingsNav(msg)
}

func (m model) updateSettingsNav(msg tea.Msg) (tea.Model, tea.Cmd) {
	count := m.settingsItemCount()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.message = ""

		switch msg.String() {
		case "esc", "q":
			m.page = pageFleetList
			return m, nil

		case "ctrl+c", "ctrl+q":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			m.settingsCursor = (m.settingsCursor - 1 + count) % count
			return m, nil

		case "down", "j":
			m.settingsCursor = (m.settingsCursor + 1) % count
			return m, nil

		case "left", "h":
			item := m.settingsCursorItem()
			if item == settingsItemToolSelection {
				m.cycleAgentTool(-1)
			} else if item == settingsItemDotfilesAutoInstall {
				m.toggleAutoInstall()
			} else if item == settingsItemCoderPreset {
				m.cycleCoderPreset(-1)
			} else if item == settingsItemCodespacesMachine {
				m.cycleCodespacesMachine(-1)
			}
			return m, nil

		case "right", "l":
			item := m.settingsCursorItem()
			if item == settingsItemToolSelection {
				m.cycleAgentTool(1)
			} else if item == settingsItemDotfilesAutoInstall {
				m.toggleAutoInstall()
			} else if item == settingsItemCoderPreset {
				m.cycleCoderPreset(1)
			} else if item == settingsItemCodespacesMachine {
				m.cycleCodespacesMachine(1)
			}
			return m, nil

		case "enter", " ":
			item := m.settingsCursorItem()
			if item == settingsItemToolSelection {
				m.cycleAgentTool(1)
				return m, nil
			}
			if item == settingsItemDotfilesAutoInstall {
				m.toggleAutoInstall()
				return m, nil
			}
			if item == settingsItemCoderPreset {
				m.cycleCoderPreset(1)
			}
			if item == settingsItemCodespacesMachine {
				m.cycleCodespacesMachine(1)
				return m, nil
			}
			if item == settingsItemDoctor {
				cmd, err := doctor.Command()
				if err != nil {
					m.message = err.Error()
					return m, nil
				}
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{err} })
			}
			if item >= settingsItemToolStatusBase {
				idx := item - settingsItemToolStatusBase
				if idx < len(m.toolStatus) {
					openURL(m.toolStatus[idx].InstallURL)
					m.message = fmt.Sprintf("Opening %s", m.toolStatus[idx].InstallURL)
				}
				return m, nil
			}
			return m.enterSettingsEditing()
		}
	}

	return m, nil
}

func (m model) enterSettingsEditing() (tea.Model, tea.Cmd) {
	if m.cfg == nil {
		m.cfg = state.DefaultConfig()
	}

	item := m.settingsCursorItem()
	var current string
	switch {
	case item == settingsItemDotfilesRepo:
		current = m.cfg.DotfilesSettings.RepoURL
		m.settingsInput.Placeholder = "https://github.com/user/dotfiles"
	case item == settingsItemDotfilesScript:
		current = m.cfg.DotfilesSettings.InstallScript
		m.settingsInput.Placeholder = "install.sh"
	case item == settingsItemCoderTemplate:
		current = m.cfg.CoderSettings.Template
		m.settingsInput.Placeholder = "template-name"
	case item >= settingsItemCoderParamBase && item < settingsItemCodespacesMachine:
		idx := item - settingsItemCoderParamBase
		if idx < len(m.cfg.CoderSettings.Parameters) {
			current = m.cfg.CoderSettings.Parameters[idx].Value
			p := m.cfg.CoderSettings.Parameters[idx]
			if p.DefaultValue != "" {
				m.settingsInput.Placeholder = p.DefaultValue
			} else {
				m.settingsInput.Placeholder = "value"
			}
		}
	case item == settingsItemCodespacesMachine:
		// Machine is a cycle selector, not a text field
		m.cycleCodespacesMachine(1)
		return m, nil
	case item == settingsItemCodespacesIdle:
		current = m.cfg.CodespacesSettings.IdleTimeout
		m.settingsInput.Placeholder = "30m"
	case item == settingsItemCodespacesDevcontainer:
		current = m.cfg.CodespacesSettings.DevcontainerPath
		m.settingsInput.Placeholder = ".devcontainer/devcontainer.json"
	default:
		return m, nil
	}

	m.settingsEditing = true
	m.settingsInput.SetValue(current)
	m.settingsInput.Focus()
	m.settingsInput.CursorEnd()
	return m, m.settingsInput.Cursor.BlinkCmd()
}

func (m model) updateSettingsEditing(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			value := strings.TrimSpace(m.settingsInput.Value())
			if m.cfg == nil {
				m.cfg = state.DefaultConfig()
			}

			item := m.settingsCursorItem()
			var cmd tea.Cmd
			switch {
			case item == settingsItemDotfilesRepo:
				m.cfg.DotfilesSettings.RepoURL = value
			case item == settingsItemDotfilesScript:
				m.cfg.DotfilesSettings.InstallScript = value
			case item == settingsItemCoderTemplate:
				oldTemplate := m.cfg.CoderSettings.Template
				m.cfg.CoderSettings.Template = value
				// If template changed, trigger parameter fetch
				if value != "" && value != oldTemplate {
					m.coderFetchingParams = true
					m.message = "Fetching template parameters..."
					cmd = fetchCoderParamsCmd(value)
				}
			case item >= settingsItemCoderParamBase && item < settingsItemCodespacesMachine:
				idx := item - settingsItemCoderParamBase
				if idx < len(m.cfg.CoderSettings.Parameters) {
					m.cfg.CoderSettings.Parameters[idx].Value = value
				}
			case item == settingsItemCodespacesIdle:
				m.cfg.CodespacesSettings.IdleTimeout = value
			case item == settingsItemCodespacesDevcontainer:
				m.cfg.CodespacesSettings.DevcontainerPath = value
			}

			if err := state.SaveConfig(m.cfg); err != nil {
				m.message = fmt.Sprintf("Failed to save settings: %v", err)
			} else if cmd == nil {
				m.message = "Saved"
			}
			m.settingsEditing = false
			m.settingsInput.Blur()
			return m, cmd

		case tea.KeyEsc:
			m.settingsEditing = false
			m.settingsInput.Blur()
			m.message = "Cancelled"
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.settingsInput, cmd = m.settingsInput.Update(msg)
	return m, cmd
}

func (m model) viewSettings() string {
	var b strings.Builder

	b.WriteString(renderGradient(nameToBanner("Settings")))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	}

	cfg := m.cfg
	if cfg == nil {
		cfg = state.DefaultConfig()
	}

	box := listBox
	if m.width > 0 {
		box = box.Width(m.width - 2)
	}

	ruleWidth := 28
	if m.width > 0 {
		ruleWidth = max(1, m.width-2-box.GetHorizontalFrameSize())
	}

	var listContent strings.Builder
	currentItem := m.settingsCursorItem()

	for _, s := range settingsSections {
		if !m.sectionVisible(s) {
			continue
		}

		// Section header
		listContent.WriteString(fleetExpandedStyle.Render(s.Title))
		listContent.WriteString("\n")
		listContent.WriteString(dimStyle.Render(strings.Repeat("─", ruleWidth)))
		listContent.WriteString("\n\n")

		// Section-specific rows
		switch s.Title {
		case "Agent Settings":
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemToolSelection,
				"Tool selection", fmt.Sprintf("[ %s ]", agentToolLabel(cfg.AgentSettings.ToolSelection))))

		case "Dotfiles":
			repoValue := cfg.DotfilesSettings.RepoURL
			if repoValue == "" && !(m.settingsEditing && currentItem == settingsItemDotfilesRepo) {
				repoValue = dimStyle.Render("(not set)")
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemDotfilesRepo, "Repository URL", repoValue))
			listContent.WriteString("\n")

			scriptValue := cfg.DotfilesSettings.InstallScript
			if scriptValue == "" && !(m.settingsEditing && currentItem == settingsItemDotfilesScript) {
				scriptValue = dimStyle.Render("(not set)")
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemDotfilesScript, "Install script", scriptValue))
			listContent.WriteString("\n")

			autoInstallValue := "[ off ]"
			if cfg.DotfilesSettings.AutoInstall {
				autoInstallValue = "[ on ]"
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemDotfilesAutoInstall, "Auto install", autoInstallValue))

		case "Coder":
			templateValue := cfg.CoderSettings.Template
			if templateValue == "" && !(m.settingsEditing && currentItem == settingsItemCoderTemplate) {
				templateValue = dimStyle.Render("(not set)")
			}
			if m.coderFetchingParams {
				templateValue += "  " + m.spinner.View() + " fetching..."
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemCoderTemplate, "Template", templateValue))
			listContent.WriteString("\n")

			presetValue := cfg.CoderSettings.Preset
			if presetValue == "" {
				presetValue = dimStyle.Render("(none)")
			} else {
				presetValue = fmt.Sprintf("[ %s ]", presetValue)
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemCoderPreset, "Preset", presetValue))

			// Dynamic parameter rows
			for i, p := range cfg.CoderSettings.Parameters {
				listContent.WriteString("\n")
				paramItem := settingsItemCoderParamBase + i
				value := p.Value
				if value == "" && !(m.settingsEditing && currentItem == paramItem) {
					if p.DefaultValue != "" {
						value = dimStyle.Render(p.DefaultValue + " (default)")
					} else {
						value = dimStyle.Render("(not set)")
					}
				}
				label := p.Name
				if p.DisplayName != "" {
					label = p.DisplayName
				}
				listContent.WriteString(m.renderSettingsRow(currentItem == paramItem, label, value))
			}

		case "Codespaces":
			var machineValue string
			if cfg.CodespacesSettings.Machine == "" {
				if m.codespaceFetchingMachines {
					machineValue = m.spinner.View() + " fetching..."
				} else {
					machineValue = dimStyle.Render("(none)")
				}
			} else {
				machineValue = fmt.Sprintf("[ %s ]", cfg.CodespacesSettings.Machine)
				if label := m.codespacesMachineLabel(); label != cfg.CodespacesSettings.Machine {
					machineValue += "\n" + strings.Repeat(" ", 21) + dimStyle.Render(label)
				}
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemCodespacesMachine, "Machine", machineValue))
			listContent.WriteString("\n")

			idleValue := cfg.CodespacesSettings.IdleTimeout
			if idleValue == "" && !(m.settingsEditing && currentItem == settingsItemCodespacesIdle) {
				idleValue = dimStyle.Render("(not set)")
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemCodespacesIdle, "Idle timeout", idleValue))
			listContent.WriteString("\n")

			dcPathValue := cfg.CodespacesSettings.DevcontainerPath
			if dcPathValue == "" && !(m.settingsEditing && currentItem == settingsItemCodespacesDevcontainer) {
				dcPathValue = dimStyle.Render("(not set)")
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemCodespacesDevcontainer, "Devcontainer path", dcPathValue))

		case "Tool Status":
			for i, t := range m.toolStatus {
				if i > 0 {
					listContent.WriteString("\n")
				}
				var badge string
				if t.Found {
					badge = statusRunningStyle.Render("installed")
				} else {
					badge = statusCreatingStyle.Render("not found")
				}
				value := badge + "  " + dimStyle.Render(t.Description)
				itemID := settingsItemToolStatusBase + i
				listContent.WriteString(m.renderSettingsRow(currentItem == itemID, t.Name, value))
			}

		case "Doctor":
			agentName, _, agentErr := doctor.FindAgent()
			var value string
			if agentErr != nil {
				value = statusCreatingStyle.Render("no agent found") + "  " + dimStyle.Render("install claude, codex, gemini, or copilot")
			} else {
				value = statusRunningStyle.Render(agentName) + "  " + dimStyle.Render("press enter to diagnose your setup")
			}
			listContent.WriteString(m.renderSettingsRow(currentItem == settingsItemDoctor, "Run Doctor", value))
		}

		listContent.WriteString("\n\n")
	}

	b.WriteString(box.Render(strings.TrimRight(listContent.String(), "\n")))
	b.WriteString("\n")

	// Substitution variable hint
	if currentItem >= settingsItemCoderParamBase && currentItem < settingsItemToolStatusBase {
		b.WriteString(dimStyle.Render("  Variables: ${GIT_URL} = fleet repo URL, ${INSTANCE_NAME} = workspace name"))
		b.WriteString("\n")
	}

	if m.message != "" {
		b.WriteString(messageStyle.Render(m.message))
		b.WriteString("\n")
	}

	if m.settingsEditing {
		b.WriteString(renderHelp(m.width, []string{
			"enter: save", "esc: cancel",
		}))
	} else {
		b.WriteString(renderHelp(m.width, []string{
			"j/k: navigate", "left/right: cycle", "enter: edit", "esc: back", "ctrl+c: quit",
		}))
	}

	return b.String()
}

func (m model) renderSettingsRow(active bool, label string, value string) string {
	cursor := "  "
	if active {
		cursor = cursorStyle.Render("> ")
	}

	formattedLabel := fmt.Sprintf("%-18s", label)

	// If editing this row, show the text input instead of the static value
	if m.settingsEditing && active {
		value = m.settingsInput.View()
	}

	if active {
		return fmt.Sprintf("%s%s %s", cursor, selectedStyle.Render(formattedLabel), value)
	}
	return fmt.Sprintf("%s%s %s", cursor, formattedLabel, value)
}

// openURL opens the given URL in the user's default browser.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
