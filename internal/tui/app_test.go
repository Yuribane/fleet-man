package tui

import (
	"testing"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateSettingsCyclesToolAndPersistsConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := model{
		page:           pageSettings,
		cfg:            state.DefaultConfig(),
		settingsCursor: settingsItemToolSelection,
		settingsInput:  textinput.New(),
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
	}

	// Down from 0 -> 1
	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(model)
	if got.settingsCursor != settingsItemDotfilesRepo {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemDotfilesRepo)
	}

	// Down from 1 -> 2
	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != settingsItemDotfilesScript {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemDotfilesScript)
	}

	// Down from 2 -> wraps to 0
	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyDown})
	got = updated.(model)
	if got.settingsCursor != settingsItemToolSelection {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemToolSelection)
	}

	// Up from 0 -> wraps to 2
	updated, _ = got.updateSettings(tea.KeyMsg{Type: tea.KeyUp})
	got = updated.(model)
	if got.settingsCursor != settingsItemDotfilesScript {
		t.Fatalf("cursor = %d, want %d", got.settingsCursor, settingsItemDotfilesScript)
	}
}

func TestUpdateSettingsEnterEditingDotfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := model{
		page:           pageSettings,
		cfg:            state.DefaultConfig(),
		settingsCursor: settingsItemDotfilesRepo,
		settingsInput:  textinput.New(),
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

func tickProbe(tool string, ticks int64) devcontainer.AgentProbeResult {
	return devcontainer.AgentProbeResult{Tool: tool, CPUTicks: ticks}
}

func stateProbe(tool, st string) devcontainer.AgentProbeResult {
	return devcontainer.AgentProbeResult{Tool: tool, State: st}
}

func TestDeriveAgentStates(t *testing.T) {
	// File-based detection tests (claude, copilot)
	t.Run("file-based working", func(t *testing.T) {
		m := model{
			agentStates:    make(map[string]agentState),
			agentPrevTicks: make(map[string]int64),
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": stateProbe("claude", "working")}, []string{"c1"})
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking", m.agentStates["c1"])
		}
	})

	t.Run("file-based waiting", func(t *testing.T) {
		m := model{
			agentStates:    make(map[string]agentState),
			agentPrevTicks: make(map[string]int64),
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": stateProbe("copilot", "waiting")}, []string{"c1"})
		if m.agentStates["c1"] != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", m.agentStates["c1"])
		}
	})

	// Tick-based detection tests (codex, gemini)
	t.Run("tick first reading assumes working", func(t *testing.T) {
		m := model{
			agentStates:    make(map[string]agentState),
			agentPrevTicks: make(map[string]int64),
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": tickProbe("codex", 500)}, []string{"c1"})
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking", m.agentStates["c1"])
		}
	})

	t.Run("tick increased means working", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWorking},
			agentPrevTicks: map[string]int64{"c1": 500},
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": tickProbe("codex", 600)}, []string{"c1"})
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking", m.agentStates["c1"])
		}
	})

	t.Run("tick unchanged means waiting", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWorking},
			agentPrevTicks: map[string]int64{"c1": 500},
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": tickProbe("codex", 500)}, []string{"c1"})
		if m.agentStates["c1"] != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", m.agentStates["c1"])
		}
	})

	t.Run("tick at threshold still waiting", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWorking},
			agentPrevTicks: map[string]int64{"c1": 500},
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": tickProbe("codex", 500+agentIdleTickThreshold)}, []string{"c1"})
		if m.agentStates["c1"] != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", m.agentStates["c1"])
		}
	})

	t.Run("tick above threshold means working", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWaiting},
			agentPrevTicks: map[string]int64{"c1": 500},
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": tickProbe("codex", 500+agentIdleTickThreshold+1)}, []string{"c1"})
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking", m.agentStates["c1"])
		}
	})

	t.Run("empty tool means not running", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWorking},
			agentPrevTicks: map[string]int64{"c1": 500},
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": {}}, []string{"c1"})
		if m.agentStates["c1"] != agentNotRunning {
			t.Fatalf("got %d, want agentNotRunning", m.agentStates["c1"])
		}
	})

	t.Run("tracks detected tool from file-based probe", func(t *testing.T) {
		m := model{
			agentStates:    make(map[string]agentState),
			agentPrevTicks: make(map[string]int64),
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": stateProbe("copilot", "working")}, []string{"c1"})
		if m.agentTools["c1"] != state.AgentToolCopilot {
			t.Fatalf("got %q, want %q", m.agentTools["c1"], state.AgentToolCopilot)
		}
	})

	t.Run("preserves state on probe failure", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWorking},
			agentTools:     map[string]state.AgentTool{"c1": state.AgentToolClaude},
			agentPrevTicks: map[string]int64{"c1": 500},
		}
		// c1 is in expectedIDs but missing from probes (probe failed)
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{}, []string{"c1"})
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking (preserved)", m.agentStates["c1"])
		}
		if m.agentTools["c1"] != state.AgentToolClaude {
			t.Fatalf("tool not preserved: got %q", m.agentTools["c1"])
		}
	})

	t.Run("cleans up removed containers", func(t *testing.T) {
		m := model{
			agentStates:    map[string]agentState{"c1": agentWorking, "c2": agentWaiting},
			agentPrevTicks: map[string]int64{"c1": 500, "c2": 300},
		}
		m.deriveAgentStates(map[string]devcontainer.AgentProbeResult{"c1": tickProbe("codex", 600)}, []string{"c1"})
		if _, ok := m.agentStates["c2"]; ok {
			t.Fatal("c2 should have been cleaned up")
		}
		if _, ok := m.agentPrevTicks["c2"]; ok {
			t.Fatal("c2 prev ticks should have been cleaned up")
		}
	})
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
