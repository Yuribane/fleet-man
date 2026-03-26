package tui

import (
	"testing"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateSettingsCyclesToolAndPersistsConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := model{
		page: pageSettings,
		cfg:  state.DefaultConfig(),
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
