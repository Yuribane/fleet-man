package tui

import (
	"testing"

	"github.com/BenjaminBenetti/fleet-man/internal/deps"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// allToolsFound returns a toolStatus slice where every tool is marked as found,
// ensuring all settings sections are visible during tests.
func allToolsFound() []deps.ToolStatus {
	return []deps.ToolStatus{
		{Name: "devcontainer", Binary: "devcontainer", Found: true},
		{Name: "gh", Binary: "gh", Found: true},
		{Name: "coder", Binary: "coder", Found: true},
	}
}

func TestUpdateSettingsCyclesToolAndPersistsConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := model{
		page:           pageSettings,
		cfg:            state.DefaultConfig(),
		settingsCursor: settingsItemToolSelection,
		settingsInput:  textinput.New(),
		toolStatus:     allToolsFound(),
	}

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(model)

	if got.cfg == nil {
		t.Fatal("cfg is nil after updateSettings")
	}
	if got.cfg.AgentSettings.ToolSelection != state.AgentToolGemini {
		t.Fatalf("ToolSelection = %q, want %q", got.cfg.AgentSettings.ToolSelection, state.AgentToolGemini)
	}

	cfg, err := state.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.AgentSettings.ToolSelection != state.AgentToolGemini {
		t.Fatalf("persisted ToolSelection = %q, want %q", cfg.AgentSettings.ToolSelection, state.AgentToolGemini)
	}
}

func TestUpdateSettingsEscReturnsToFleetList(t *testing.T) {
	m := model{page: pageSettings}

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(model)

	if got.page != pageFleetList {
		t.Fatalf("page = %v, want %v", got.page, pageFleetList)
	}
}

func TestUpdateNormalWrapsCursorFromTopToBottom(t *testing.T) {
	m := model{
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha"},
			{kind: rowSettings},
		},
		cursor: 0,
	}

	updated, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyUp})
	got := updated.(model)

	if got.cursor != len(got.rows)-1 {
		t.Fatalf("cursor = %d, want %d", got.cursor, len(got.rows)-1)
	}
}

func TestUpdateNormalWrapsCursorFromBottomToTop(t *testing.T) {
	m := model{
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha"},
			{kind: rowSettings},
		},
		cursor: 2,
	}

	updated, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(model)

	if got.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", got.cursor)
	}
}

func TestUpdateSettingsNavUpDown(t *testing.T) {
	m := model{
		page:           pageSettings,
		cfg:            state.DefaultConfig(),
		settingsCursor: settingsItemToolSelection,
		settingsInput:  textinput.New(),
		toolStatus:     allToolsFound(),
	}

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(model)
	if got.settingsCursor != settingsItemDotfilesRepo {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemDotfilesRepo)
	}

	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != settingsItemDotfilesScript {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemDotfilesScript)
	}

	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != settingsItemDotfilesAutoInstall {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemDotfilesAutoInstall)
	}

	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != settingsItemCoderTemplate {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemCoderTemplate)
	}

	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != settingsItemCoderPreset {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemCoderPreset)
	}

	// Navigate through tool status rows
	for i := 0; i < 3; i++ {
		updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
		got = updated.(model)
	}

	// Navigate through doctor row
	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)

	// Wrap past last item back to first
	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != 0 {
		t.Fatalf("cursor = %d, want 0 (wrap to top)", got.settingsCursor)
	}

	// Wrap up from first goes to last tool status item
	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyUp})
	got = updated.(model)
	wantLast := got.settingsItemCount() - 1
	if got.settingsCursor != wantLast {
		t.Fatalf("cursor = %d, want %d (wrap to bottom)", got.settingsCursor, wantLast)
	}
}

func TestUpdateSettingsEnterEditingDotfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := model{
		page:           pageSettings,
		cfg:            state.DefaultConfig(),
		settingsCursor: settingsItemDotfilesRepo,
		settingsInput:  textinput.New(),
		toolStatus:     allToolsFound(),
	}

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if !got.settingsEditing {
		t.Fatal("settingsEditing should be true after enter on dotfiles repo")
	}
	if !got.settingsInput.Focused() {
		t.Fatal("settingsInput should be focused")
	}
}

func TestUpdateSettingsEditingSavesOnEnter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	si := textinput.New()
	si.CharLimit = 256

	m := model{
		page:            pageSettings,
		cfg:             state.DefaultConfig(),
		settingsCursor:  settingsItemDotfilesRepo,
		settingsEditing: true,
		settingsInput:   si,
		toolStatus:      allToolsFound(),
	}
	m.settingsInput.SetValue("https://github.com/user/dotfiles")
	m.settingsInput.Focus()

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if got.settingsEditing {
		t.Fatal("settingsEditing should be false after enter")
	}
	if got.cfg.DotfilesSettings.RepoURL != "https://github.com/user/dotfiles" {
		t.Fatalf("RepoURL = %q, want %q", got.cfg.DotfilesSettings.RepoURL, "https://github.com/user/dotfiles")
	}

	cfg, err := state.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.DotfilesSettings.RepoURL != "https://github.com/user/dotfiles" {
		t.Fatalf("persisted RepoURL = %q, want %q", cfg.DotfilesSettings.RepoURL, "https://github.com/user/dotfiles")
	}
}

func TestUpdateSettingsEditingCancelsOnEsc(t *testing.T) {
	si := textinput.New()
	si.CharLimit = 256

	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "original"

	m := model{
		page:            pageSettings,
		cfg:             cfg,
		settingsCursor:  settingsItemDotfilesRepo,
		settingsEditing: true,
		settingsInput:   si,
		toolStatus:      allToolsFound(),
	}
	m.settingsInput.SetValue("changed")
	m.settingsInput.Focus()

	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(model)

	if got.settingsEditing {
		t.Fatal("settingsEditing should be false after esc")
	}
	if got.cfg.DotfilesSettings.RepoURL != "original" {
		t.Fatalf("RepoURL = %q, want %q (should not have changed)", got.cfg.DotfilesSettings.RepoURL, "original")
	}
}
