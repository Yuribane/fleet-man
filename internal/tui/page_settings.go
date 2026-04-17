package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/agent"
	"github.com/BenjaminBenetti/fleet-man/internal/doctor"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ===========================================
// Settings Item Constants
// ===========================================

const (
	settingsItemToolSelection = iota
	settingsItemTmuxVimKeys
	settingsItemShowHelpText
	settingsItemUpdate // only visible when an update is available
	settingsItemDotfilesRepo
	settingsItemDotfilesScript
	settingsItemDotfilesAutoInstall
	settingsItemDotfilesSetup
	settingsItemCoderTemplate
	settingsItemCoderPreset
	settingsItemCoderParamBase // parameters are at index base + i

	settingsItemCodespacesMachine = 500 // codespaces settings start here

	settingsItemToolStatusBase = 1000 // tool status rows start here
	settingsItemDoctor         = 2000 // doctor action row
	settingsItemKeybindings    = 2001 // keybindings dialog row
)

// toolStatusCount is the number of rows rendered in the Tool Status
// section. Must match the length of deps.CheckTools().
const toolStatusCount = 5

// dotfilesSetupPrompt is the instruction sent to the coding agent for
// guided dotfiles setup.
const dotfilesSetupPrompt = "Follow the instructions in https://raw.githubusercontent.com/BenjaminBenetti/Teeleport/main/SETUP_SKILL.md to help me set up Teeleport."

// ===========================================
// Settings Page
// ===========================================

// settingsPage holds settings-page-specific state.
type settingsPage struct {
	cursor          int
	editing         bool
	input           textinput.Model
	showKeybindings bool
}

// newSettingsPage creates a new settings page with default state.
func newSettingsPage() *settingsPage {
	si := textinput.New()
	si.CharLimit = 256
	return &settingsPage{
		input: si,
	}
}

// Init is called when the settings page becomes active.
func (sp *settingsPage) Init(m *model) tea.Cmd {
	return nil
}

// Update dispatches settings page messages to the appropriate handler.
func (sp *settingsPage) Update(m *model, msg tea.Msg) tea.Cmd {
	if sp.showKeybindings {
		return sp.updateKeybindingsDialog(m, msg)
	}
	if sp.editing {
		return sp.updateSettingsEditing(m, msg)
	}
	return sp.updateSettingsNav(m, msg)
}

// View renders the settings page.
func (sp *settingsPage) View(m *model) string {
	return sp.viewSettings(m)
}

// ===========================================
// Settings Sections
// ===========================================

// settingsSection defines a titled group of settings rows that can be
// conditionally shown based on tool availability.
type settingsSection struct {
	Title string                        // section header text
	Tool  string                        // required tool binary; "" = always visible
	Items func(cfg *state.Config) []int // returns navigable item IDs for this section
}

// settingsSections lists all settings sections in display order.
var settingsSections = []settingsSection{
	{
		Title: "General",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemTmuxVimKeys, settingsItemShowHelpText, settingsItemUpdate}
		},
	},
	{
		Title: "Dotfiles",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemDotfilesRepo, settingsItemDotfilesScript, settingsItemDotfilesAutoInstall, settingsItemDotfilesSetup}
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
			return []int{settingsItemCodespacesMachine}
		},
	},
	{
		Title: "Tool Status",
		Items: func(_ *state.Config) []int {
			items := make([]int, toolStatusCount)
			for i := range items {
				items[i] = settingsItemToolStatusBase + i
			}
			return items
		},
	},
	{
		Title: "Help",
		Items: func(_ *state.Config) []int {
			return []int{settingsItemDoctor, settingsItemKeybindings}
		},
	},
}

// ===========================================
// Navigation Helpers
// ===========================================

// sectionVisible reports whether a settings section should be shown.
func (sp *settingsPage) sectionVisible(m *model, s settingsSection) bool {
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

// visibleItems returns the flat ordered list of navigable item IDs.
func (sp *settingsPage) visibleItems(m *model) []int {
	var items []int
	for _, s := range settingsSections {
		if !sp.sectionVisible(m, s) {
			continue
		}
		for _, id := range s.Items(m.cfg) {
			if id == settingsItemUpdate && m.updateAvailable == "" {
				continue
			}
			items = append(items, id)
		}
	}
	return items
}

// settingsCursorItem returns the item ID at the current cursor position.
func (sp *settingsPage) settingsCursorItem(m *model) int {
	items := sp.visibleItems(m)
	if sp.cursor >= 0 && sp.cursor < len(items) {
		return items[sp.cursor]
	}
	return -1
}

// settingsItemCount returns the total number of navigable settings rows.
func (sp *settingsPage) settingsItemCount(m *model) int {
	return len(sp.visibleItems(m))
}

// ===========================================
// Toggle Helpers
// ===========================================

// toggleTmuxVimKeys toggles the tmux vim keys setting.
func (sp *settingsPage) toggleTmuxVimKeys(m *model) {
	if m.cfg == nil {
		m.cfg = state.DefaultConfig()
	}
	current := m.cfg.GeneralSettings.TmuxVimKeysEnabled()
	next := !current
	m.cfg.GeneralSettings.TmuxVimKeys = &next
	if err := state.SaveConfig(m.cfg); err != nil {
		m.cfg.GeneralSettings.TmuxVimKeys = &current
		m.message = fmt.Sprintf("Failed to save settings: %v", err)
		return
	}
	label := "off"
	if next {
		label = "on"
	}
	m.message = fmt.Sprintf("Tmux vim keys set to %s", label)
}

// toggleShowHelpText flips the show-help-text preference and saves.
func (sp *settingsPage) toggleShowHelpText(m *model) {
	if m.cfg == nil {
		m.cfg = state.DefaultConfig()
	}
	current := m.cfg.GeneralSettings.ShowHelpTextEnabled()
	next := !current
	m.cfg.GeneralSettings.ShowHelpText = &next
	if err := state.SaveConfig(m.cfg); err != nil {
		m.cfg.GeneralSettings.ShowHelpText = &current
		m.message = fmt.Sprintf("Failed to save settings: %v", err)
		return
	}
	label := "off"
	if next {
		label = "on"
	}
	m.message = fmt.Sprintf("Show help text set to %s", label)
}

// toggleAutoInstall toggles the dotfiles auto-install setting.
func (sp *settingsPage) toggleAutoInstall(m *model) {
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

// cycleCoderPreset cycles through available coder presets.
func (sp *settingsPage) cycleCoderPreset(m *model, direction int) {
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

// cycleCodespacesMachine cycles through available codespace machine types.
func (sp *settingsPage) cycleCodespacesMachine(m *model, direction int) {
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
// configured machine.
func (sp *settingsPage) codespacesMachineLabel(m *model) string {
	name := m.cfg.CodespacesSettings.Machine
	for _, mt := range m.codespaceMachines {
		if mt.Name == name {
			return mt.DisplayName
		}
	}
	return name
}

// ===========================================
// Update Handlers
// ===========================================

// updateKeybindingsDialog handles input while the keybindings overlay is shown.
func (sp *settingsPage) updateKeybindingsDialog(m *model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			sp.showKeybindings = false
		}
	}
	return nil
}

// updateSettingsNav handles keyboard navigation in the settings page.
func (sp *settingsPage) updateSettingsNav(m *model, msg tea.Msg) tea.Cmd {
	count := sp.settingsItemCount(m)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.message = ""

		switch msg.String() {
		case "esc", "q":
			return m.ChangeRoute(routeFleetList)

		case "ctrl+c", "ctrl+q":
			m.quitting = true
			return tea.Quit

		case "up", "k":
			sp.cursor = (sp.cursor - 1 + count) % count
			return nil

		case "down", "j":
			sp.cursor = (sp.cursor + 1) % count
			return nil

		case "left", "h":
			item := sp.settingsCursorItem(m)
			if item == settingsItemTmuxVimKeys {
				sp.toggleTmuxVimKeys(m)
			} else if item == settingsItemShowHelpText {
				sp.toggleShowHelpText(m)
			} else if item == settingsItemDotfilesAutoInstall {
				sp.toggleAutoInstall(m)
			} else if item == settingsItemCoderPreset {
				sp.cycleCoderPreset(m, -1)
			} else if item == settingsItemCodespacesMachine {
				sp.cycleCodespacesMachine(m, -1)
			}
			return nil

		case "right", "l":
			item := sp.settingsCursorItem(m)
			if item == settingsItemTmuxVimKeys {
				sp.toggleTmuxVimKeys(m)
			} else if item == settingsItemShowHelpText {
				sp.toggleShowHelpText(m)
			} else if item == settingsItemDotfilesAutoInstall {
				sp.toggleAutoInstall(m)
			} else if item == settingsItemCoderPreset {
				sp.cycleCoderPreset(m, 1)
			} else if item == settingsItemCodespacesMachine {
				sp.cycleCodespacesMachine(m, 1)
			}
			return nil

		case "enter", " ":
			item := sp.settingsCursorItem(m)
			if item == settingsItemTmuxVimKeys {
				sp.toggleTmuxVimKeys(m)
				return nil
			}
			if item == settingsItemShowHelpText {
				sp.toggleShowHelpText(m)
				return nil
			}
			if item == settingsItemDotfilesAutoInstall {
				sp.toggleAutoInstall(m)
				return nil
			}
			if item == settingsItemCoderPreset {
				sp.cycleCoderPreset(m, 1)
			}
			if item == settingsItemCodespacesMachine {
				sp.cycleCodespacesMachine(m, 1)
				return nil
			}
			if item == settingsItemUpdate {
				return performUpdateCmd(m.inHostTmux)
			}
			if item == settingsItemDoctor {
				cmd, err := doctor.Command()
				if err != nil {
					m.message = err.Error()
					return nil
				}
				return tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{err} })
			}
			if item == settingsItemKeybindings {
				sp.showKeybindings = true
				return nil
			}
			if item == settingsItemDotfilesSetup {
				cmd, err := agent.CommandWithPrompt(dotfilesSetupPrompt)
				if err != nil {
					m.message = err.Error()
					return nil
				}
				return tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{err} })
			}
			if item >= settingsItemToolStatusBase {
				idx := item - settingsItemToolStatusBase
				if idx < len(m.toolStatus) {
					openURL(m.toolStatus[idx].InstallURL)
					m.message = fmt.Sprintf("Opening %s", m.toolStatus[idx].InstallURL)
				}
				return nil
			}
			return sp.enterSettingsEditing(m)
		}
	}

	return nil
}

// enterSettingsEditing activates text editing for the current setting.
func (sp *settingsPage) enterSettingsEditing(m *model) tea.Cmd {
	if m.cfg == nil {
		m.cfg = state.DefaultConfig()
	}

	item := sp.settingsCursorItem(m)
	var current string
	switch {
	case item == settingsItemDotfilesRepo:
		current = m.cfg.DotfilesSettings.RepoURL
		sp.input.Placeholder = "https://github.com/user/dotfiles"
	case item == settingsItemDotfilesScript:
		current = m.cfg.DotfilesSettings.InstallScript
		sp.input.Placeholder = "install.sh"
	case item == settingsItemCoderTemplate:
		current = m.cfg.CoderSettings.Template
		sp.input.Placeholder = "template-name"
	case item >= settingsItemCoderParamBase && item < settingsItemCodespacesMachine:
		idx := item - settingsItemCoderParamBase
		if idx < len(m.cfg.CoderSettings.Parameters) {
			current = m.cfg.CoderSettings.Parameters[idx].Value
			p := m.cfg.CoderSettings.Parameters[idx]
			if p.DefaultValue != "" {
				sp.input.Placeholder = p.DefaultValue
			} else {
				sp.input.Placeholder = "value"
			}
		}
	case item == settingsItemCodespacesMachine:
		sp.cycleCodespacesMachine(m, 1)
		return nil
	default:
		return nil
	}

	sp.editing = true
	sp.input.SetValue(current)
	sp.input.Focus()
	sp.input.CursorEnd()
	return sp.input.Cursor.BlinkCmd()
}

// updateSettingsEditing handles input while editing a text field.
func (sp *settingsPage) updateSettingsEditing(m *model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			value := strings.TrimSpace(sp.input.Value())
			if m.cfg == nil {
				m.cfg = state.DefaultConfig()
			}

			item := sp.settingsCursorItem(m)
			var cmd tea.Cmd
			switch {
			case item == settingsItemDotfilesRepo:
				m.cfg.DotfilesSettings.RepoURL = value
			case item == settingsItemDotfilesScript:
				m.cfg.DotfilesSettings.InstallScript = value
			case item == settingsItemCoderTemplate:
				oldTemplate := m.cfg.CoderSettings.Template
				m.cfg.CoderSettings.Template = value
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
			}

			if err := state.SaveConfig(m.cfg); err != nil {
				m.message = fmt.Sprintf("Failed to save settings: %v", err)
			} else if cmd == nil {
				m.message = "Saved"
			}
			sp.editing = false
			sp.input.Blur()
			return cmd

		case tea.KeyEsc:
			sp.editing = false
			sp.input.Blur()
			m.message = "Cancelled"
			return nil
		}
	}

	var cmd tea.Cmd
	sp.input, cmd = sp.input.Update(msg)
	return cmd
}

// ===========================================
// View
// ===========================================

// viewSettings renders the settings page.
func (sp *settingsPage) viewSettings(m *model) string {
	var b strings.Builder

	b.WriteString(renderGradient(nameToBanner("Settings")))
	if m.updateAvailable != "" {
		b.WriteString("  " + updateStyle.Render(fmt.Sprintf("A new version: %s is available ⚡ ", m.updateAvailable)))
	}
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
	currentItem := sp.settingsCursorItem(m)

	for _, s := range settingsSections {
		if !sp.sectionVisible(m, s) {
			continue
		}

		listContent.WriteString(fleetExpandedStyle.Render(s.Title))
		listContent.WriteString("\n")
		listContent.WriteString(dimStyle.Render(strings.Repeat("─", ruleWidth)))
		listContent.WriteString("\n\n")

		switch s.Title {
		case "General":
			vimKeysValue := "[ off ]"
			if cfg.GeneralSettings.TmuxVimKeysEnabled() {
				vimKeysValue = "[ on ]"
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemTmuxVimKeys, "Tmux vim keys", vimKeysValue))
			listContent.WriteString("\n")

			helpTextValue := "[ off ]"
			if cfg.GeneralSettings.ShowHelpTextEnabled() {
				helpTextValue = "[ on ]"
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemShowHelpText, "Show help text", helpTextValue))

			if m.updateAvailable != "" {
				listContent.WriteString("\n")
				updateValue := updateStyle.Render(m.updateAvailable+" available ⚡") + "  " + dimStyle.Render("press enter to update")
				listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemUpdate, "Update", updateValue))
			}

		case "Dotfiles":
			repoValue := cfg.DotfilesSettings.RepoURL
			if repoValue == "" && !(sp.editing && currentItem == settingsItemDotfilesRepo) {
				repoValue = dimStyle.Render("(not set)")
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemDotfilesRepo, "Repository URL", repoValue))
			listContent.WriteString("\n")

			scriptValue := cfg.DotfilesSettings.InstallScript
			if scriptValue == "" && !(sp.editing && currentItem == settingsItemDotfilesScript) {
				scriptValue = dimStyle.Render("(not set)")
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemDotfilesScript, "Install script", scriptValue))
			listContent.WriteString("\n")

			autoInstallValue := "[ off ]"
			if cfg.DotfilesSettings.AutoInstall {
				autoInstallValue = "[ on ]"
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemDotfilesAutoInstall, "Auto install", autoInstallValue))
			listContent.WriteString("\n")

			agentName, _, agentErr := agent.FindAgent()
			var setupValue string
			if agentErr != nil {
				setupValue = statusCreatingStyle.Render("no agent found") + "  " + dimStyle.Render("install claude, codex, gemini, or copilot")
			} else {
				setupValue = statusRunningStyle.Render(agentName) + "  " + dimStyle.Render("press enter to get help setting up dotfiles")
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemDotfilesSetup, "Help me set this up", setupValue))

		case "Coder":
			templateValue := cfg.CoderSettings.Template
			if templateValue == "" && !(sp.editing && currentItem == settingsItemCoderTemplate) {
				templateValue = dimStyle.Render("(not set)")
			}
			if m.coderFetchingParams {
				templateValue += "  " + m.spinner.View() + " fetching..."
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemCoderTemplate, "Template", templateValue))
			listContent.WriteString("\n")

			presetValue := cfg.CoderSettings.Preset
			if presetValue == "" {
				presetValue = dimStyle.Render("(none)")
			} else {
				presetValue = fmt.Sprintf("[ %s ]", presetValue)
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemCoderPreset, "Preset", presetValue))

			for i, p := range cfg.CoderSettings.Parameters {
				listContent.WriteString("\n")
				paramItem := settingsItemCoderParamBase + i
				value := p.Value
				if value == "" && !(sp.editing && currentItem == paramItem) {
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
				listContent.WriteString(sp.renderSettingsRow(m, currentItem == paramItem, label, value))
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
				if label := sp.codespacesMachineLabel(m); label != cfg.CodespacesSettings.Machine {
					machineValue += "\n" + strings.Repeat(" ", 21) + dimStyle.Render(label)
				}
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemCodespacesMachine, "Machine", machineValue))

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
				listContent.WriteString(sp.renderSettingsRow(m, currentItem == itemID, t.Name, value))
			}

		case "Help":
			agentName, _, agentErr := doctor.FindAgent()
			var value string
			if agentErr != nil {
				value = statusCreatingStyle.Render("no agent found") + "  " + dimStyle.Render("install claude, codex, gemini, or copilot")
			} else {
				value = statusRunningStyle.Render(agentName) + "  " + dimStyle.Render("press enter to diagnose your setup")
			}
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemDoctor, "Run Doctor", value))
			listContent.WriteString("\n")
			listContent.WriteString(sp.renderSettingsRow(m, currentItem == settingsItemKeybindings, "Keybindings", dimStyle.Render("press enter to view all keybindings")))
		}

		listContent.WriteString("\n\n")
	}

	b.WriteString(box.Render(strings.TrimRight(listContent.String(), "\n")))
	b.WriteString("\n")

	if sp.showKeybindings {
		b.WriteString("\n")
		b.WriteString(keybindingsDialogBox.Render(sp.renderKeybindingsDialog()))
		b.WriteString("\n")
	}

	if currentItem >= settingsItemCoderParamBase && currentItem < settingsItemToolStatusBase {
		b.WriteString(dimStyle.Render("  Variables: ${GIT_URL} = fleet repo URL, ${INSTANCE_NAME} = workspace name"))
		b.WriteString("\n")
	}

	if m.message != "" {
		b.WriteString(messageStyle.Render(m.message))
		b.WriteString("\n")
	}

	if sp.editing {
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

// renderSettingsRow renders a single settings row with optional cursor
// and editing state.
func (sp *settingsPage) renderSettingsRow(m *model, active bool, label string, value string) string {
	cursor := "  "
	if active {
		cursor = cursorStyle.Render("> ")
	}

	formattedLabel := fmt.Sprintf("%-18s", label)

	if sp.editing && active {
		value = sp.input.View()
	}

	if active {
		return fmt.Sprintf("%s%s %s", cursor, selectedStyle.Render(formattedLabel), value)
	}
	return fmt.Sprintf("%s%s %s", cursor, formattedLabel, value)
}

// ===========================================
// Keybindings
// ===========================================

// keybindingEntry represents a single key/description row.
type keybindingEntry struct {
	Key  string
	Desc string
}

// keybindingGroup is a titled group of keybinding entries.
type keybindingGroup struct {
	Title   string
	Entries []keybindingEntry
}

// keybindingsData returns all application keybindings grouped by context.
func keybindingsData() []keybindingGroup {
	return []keybindingGroup{
		{
			Title: "Global",
			Entries: []keybindingEntry{
				{"ctrl+c / ctrl+q", "Quit application"},
			},
		},
		{
			Title: "Fleet List",
			Entries: []keybindingEntry{
				{"j / k", "Navigate up / down"},
				{"space / tab", "Expand / collapse"},
				{"enter / e", "Open shell or settings"},
				{"s", "Start / stop instance"},
				{"a", "Add instance"},
				{"n", "New fleet"},
				{"d", "Delete fleet / instance"},
				{"t", "Tag instance"},
				{"f", "Port forward"},
				{"c", "Open VS Code"},
				{"o", "Open external terminal"},
				{"l", "View logs"},
				{"r", "Refresh / rename session"},
			},
		},
		{
			Title: "Settings",
			Entries: []keybindingEntry{
				{"j / k", "Navigate up / down"},
				{"left / right", "Cycle option value"},
				{"enter", "Edit / toggle setting"},
				{"esc / q", "Back to fleet list"},
			},
		},
		{
			Title: "Tmux (inner session)",
			Entries: []keybindingEntry{
				{"ctrl+x", "Prefix key (nested)"},
				{"ctrl+PageUp/Down", "Cycle sessions"},
				{"prefix + T", "New tmux session"},
				{"ctrl+q / ctrl+o", "Detach from session"},
				{"j / k (vim mode)", "Select pane down / up"},
			},
		},
		{
			Title: "Tmux (outer / host)",
			Entries: []keybindingEntry{
				{"ctrl+b", "Prefix key (default)"},
				{"h / l", "Navigate panes left / right"},
			},
		},
		{
			Title: "Dialogs",
			Entries: []keybindingEntry{
				{"enter", "Confirm action"},
				{"esc", "Cancel / close"},
				{"y / n", "Yes / no (delete)"},
				{"tab", "Cycle backend (add)"},
			},
		},
	}
}

// renderKeybindingColumn renders a slice of keybinding groups into a
// single column string.
func renderKeybindingColumn(groups []keybindingGroup) string {
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(keybindingSectionStyle.Render(g.Title))
		b.WriteString("\n")
		for _, e := range g.Entries {
			b.WriteString(keybindingKeyStyle.Render(e.Key))
			b.WriteString(keybindingDescStyle.Render(e.Desc))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderKeybindingsDialog builds the content for the keybindings overlay.
func (sp *settingsPage) renderKeybindingsDialog() string {
	var b strings.Builder

	b.WriteString(dialogTitle.Render("Keybindings"))
	b.WriteString("\n\n")

	groups := keybindingsData()

	var totalLines int
	for _, g := range groups {
		totalLines += len(g.Entries) + 2
	}
	half := totalLines / 2
	splitAt := len(groups)
	running := 0
	for i, g := range groups {
		running += len(g.Entries) + 2
		if running >= half {
			splitAt = i + 1
			break
		}
	}

	left := renderKeybindingColumn(groups[:splitAt])
	right := renderKeybindingColumn(groups[splitAt:])

	colWidth := 48
	leftCol := lipgloss.NewStyle().Width(colWidth).Render(left)
	rightCol := lipgloss.NewStyle().Width(colWidth).Render(right)

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol))

	b.WriteString("\n")
	b.WriteString(dialogHint.Render("[esc] Close"))

	return b.String()
}

// ===========================================
// URL Helper
// ===========================================

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
