package tui

import (
	"fmt"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

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

func (m model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.message = ""

		switch msg.String() {
		case "esc", "q":
			m.page = pageFleetList
			return m, nil

		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "left", "h":
			m.cycleAgentTool(-1)
			return m, nil

		case "right", "l", "enter", " ":
			m.cycleAgentTool(1)
			return m, nil
		}
	}

	return m, nil
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
	listContent.WriteString(fleetExpandedStyle.Render("Agent Settings"))
	listContent.WriteString("\n")
	listContent.WriteString(dimStyle.Render(strings.Repeat("─", ruleWidth)))
	listContent.WriteString("\n\n")

	label := fmt.Sprintf("%-18s", "Tool selection")
	value := fmt.Sprintf("[ %s ]", agentToolLabel(cfg.AgentSettings.ToolSelection))
	listContent.WriteString(fmt.Sprintf("%s%s %s",
		cursorStyle.Render("> "),
		selectedStyle.Render(label),
		selectedStyle.Render(value),
	))

	b.WriteString(box.Render(strings.TrimRight(listContent.String(), "\n")))
	b.WriteString("\n")

	if m.message != "" {
		b.WriteString(messageStyle.Render(m.message))
		b.WriteString("\n")
	}

	b.WriteString(renderHelp(m.width, []string{
		"left/right: change tool", "enter: next tool", "esc: back", "q: back", "ctrl+c: quit",
	}))

	return b.String()
}
