package tui

import (
	"fmt"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	settingsItemToolSelection = iota
	settingsItemDotfilesRepo
	settingsItemDotfilesScript
	settingsItemCoderTemplate
	settingsItemCoderPreset
	settingsItemCoderParamBase // parameters are at index base + i
)

// settingsItemCount returns the total number of navigable settings rows.
func (m model) settingsItemCount() int {
	n := settingsItemCoderParamBase
	if m.cfg != nil {
		n += len(m.cfg.CoderSettings.Parameters)
	}
	return n
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
			if m.settingsCursor == settingsItemToolSelection {
				m.cycleAgentTool(-1)
			} else if m.settingsCursor == settingsItemCoderPreset {
				m.cycleCoderPreset(-1)
			}
			return m, nil

		case "right", "l":
			if m.settingsCursor == settingsItemToolSelection {
				m.cycleAgentTool(1)
			} else if m.settingsCursor == settingsItemCoderPreset {
				m.cycleCoderPreset(1)
			}
			return m, nil

		case "enter", " ":
			if m.settingsCursor == settingsItemToolSelection {
				m.cycleAgentTool(1)
				return m, nil
			}
			if m.settingsCursor == settingsItemCoderPreset {
				m.cycleCoderPreset(1)
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

	var current string
	switch {
	case m.settingsCursor == settingsItemDotfilesRepo:
		current = m.cfg.DotfilesSettings.RepoURL
		m.settingsInput.Placeholder = "https://github.com/user/dotfiles"
	case m.settingsCursor == settingsItemDotfilesScript:
		current = m.cfg.DotfilesSettings.InstallScript
		m.settingsInput.Placeholder = "install.sh"
	case m.settingsCursor == settingsItemCoderTemplate:
		current = m.cfg.CoderSettings.Template
		m.settingsInput.Placeholder = "template-name"
	case m.settingsCursor >= settingsItemCoderParamBase:
		idx := m.settingsCursor - settingsItemCoderParamBase
		if idx < len(m.cfg.CoderSettings.Parameters) {
			current = m.cfg.CoderSettings.Parameters[idx].Value
			p := m.cfg.CoderSettings.Parameters[idx]
			if p.DefaultValue != "" {
				m.settingsInput.Placeholder = p.DefaultValue
			} else {
				m.settingsInput.Placeholder = "value"
			}
		}
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

			var cmd tea.Cmd
			switch {
			case m.settingsCursor == settingsItemDotfilesRepo:
				m.cfg.DotfilesSettings.RepoURL = value
			case m.settingsCursor == settingsItemDotfilesScript:
				m.cfg.DotfilesSettings.InstallScript = value
			case m.settingsCursor == settingsItemCoderTemplate:
				oldTemplate := m.cfg.CoderSettings.Template
				m.cfg.CoderSettings.Template = value
				// If template changed, trigger parameter fetch
				if value != "" && value != oldTemplate {
					m.coderFetchingParams = true
					m.message = "Fetching template parameters..."
					cmd = fetchCoderParamsCmd(value)
				}
			case m.settingsCursor >= settingsItemCoderParamBase:
				idx := m.settingsCursor - settingsItemCoderParamBase
				if idx < len(m.cfg.CoderSettings.Parameters) {
					m.cfg.CoderSettings.Parameters[idx].Value = value
				}
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

	// Agent Settings section
	listContent.WriteString(fleetExpandedStyle.Render("Agent Settings"))
	listContent.WriteString("\n")
	listContent.WriteString(dimStyle.Render(strings.Repeat("─", ruleWidth)))
	listContent.WriteString("\n\n")
	listContent.WriteString(m.renderSettingsRow(settingsItemToolSelection, "Tool selection",
		fmt.Sprintf("[ %s ]", agentToolLabel(cfg.AgentSettings.ToolSelection))))
	listContent.WriteString("\n\n")

	// Dotfiles section
	listContent.WriteString(fleetExpandedStyle.Render("Dotfiles"))
	listContent.WriteString("\n")
	listContent.WriteString(dimStyle.Render(strings.Repeat("─", ruleWidth)))
	listContent.WriteString("\n\n")

	repoValue := cfg.DotfilesSettings.RepoURL
	if repoValue == "" && !(m.settingsEditing && m.settingsCursor == settingsItemDotfilesRepo) {
		repoValue = dimStyle.Render("(not set)")
	}
	listContent.WriteString(m.renderSettingsRow(settingsItemDotfilesRepo, "Repository URL", repoValue))
	listContent.WriteString("\n")

	scriptValue := cfg.DotfilesSettings.InstallScript
	if scriptValue == "" && !(m.settingsEditing && m.settingsCursor == settingsItemDotfilesScript) {
		scriptValue = dimStyle.Render("(not set)")
	}
	listContent.WriteString(m.renderSettingsRow(settingsItemDotfilesScript, "Install script", scriptValue))
	listContent.WriteString("\n\n")

	// Coder section
	listContent.WriteString(fleetExpandedStyle.Render("Coder"))
	listContent.WriteString("\n")
	listContent.WriteString(dimStyle.Render(strings.Repeat("─", ruleWidth)))
	listContent.WriteString("\n\n")

	templateValue := cfg.CoderSettings.Template
	if templateValue == "" && !(m.settingsEditing && m.settingsCursor == settingsItemCoderTemplate) {
		templateValue = dimStyle.Render("(not set)")
	}
	if m.coderFetchingParams {
		templateValue += "  " + m.spinner.View() + " fetching..."
	}
	listContent.WriteString(m.renderSettingsRow(settingsItemCoderTemplate, "Template", templateValue))
	listContent.WriteString("\n")

	presetValue := cfg.CoderSettings.Preset
	if presetValue == "" {
		presetValue = dimStyle.Render("(none)")
	} else {
		presetValue = fmt.Sprintf("[ %s ]", presetValue)
	}
	listContent.WriteString(m.renderSettingsRow(settingsItemCoderPreset, "Preset", presetValue))

	// Dynamic parameter rows
	for i, p := range cfg.CoderSettings.Parameters {
		listContent.WriteString("\n")
		idx := settingsItemCoderParamBase + i
		value := p.Value
		if value == "" && !(m.settingsEditing && m.settingsCursor == idx) {
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
		listContent.WriteString(m.renderSettingsRow(idx, label, value))
	}

	b.WriteString(box.Render(strings.TrimRight(listContent.String(), "\n")))
	b.WriteString("\n")

	// Substitution variable hint
	if m.settingsCursor >= settingsItemCoderParamBase {
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

func (m model) renderSettingsRow(idx int, label string, value string) string {
	isSelected := m.settingsCursor == idx

	cursor := "  "
	if isSelected {
		cursor = cursorStyle.Render("> ")
	}

	formattedLabel := fmt.Sprintf("%-18s", label)

	// If editing this row, show the text input instead of the static value
	if m.settingsEditing && isSelected {
		value = m.settingsInput.View()
	}

	if isSelected {
		return fmt.Sprintf("%s%s %s", cursor, selectedStyle.Render(formattedLabel), value)
	}
	return fmt.Sprintf("%s%s %s", cursor, formattedLabel, value)
}
