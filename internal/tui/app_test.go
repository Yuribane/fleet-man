package tui

import (
	"testing"
	"time"

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

// Screen-diff based agent detection tests

func capture(content string) devcontainer.ScreenCapture {
	return devcontainer.ScreenCapture{Content: content, OK: true}
}

func TestCountDiffs(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"identical", "hello", "hello", 0},
		{"one diff", "hello", "hallo", 1},
		{"all diff", "abc", "xyz", 3},
		{"different lengths", "ab", "abcde", 3},
		{"empty vs content", "", "hello", 5},
		{"both empty", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countDiffs(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("countDiffs(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestDetectTool(t *testing.T) {
	tests := []struct {
		name   string
		screen string
		want   state.AgentTool
	}{
		{"claude prompt", "❯ claude --resume abc123\nsome output", state.AgentToolClaude},
		{"codex session", "codex> reading files...", state.AgentToolCodex},
		{"copilot ui", "GitHub Copilot is thinking...", state.AgentToolCopilot},
		{"gemini output", "gemini: summarizing results", state.AgentToolGemini},
		{"case insensitive", "CLAUDE is working", state.AgentToolClaude},
		{"empty screen", "", ""},
		{"no tool", "vscode ➜ /workspaces $", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTool(tt.screen)
			if got != tt.want {
				t.Fatalf("detectTool(%q) = %q, want %q", tt.screen, got, tt.want)
			}
		})
	}
}

func TestDeriveAgentStates(t *testing.T) {
	now := time.Now()

	t.Run("screen changed above threshold marks working", func(t *testing.T) {
		m := model{
			agentStates:     make(map[string]agentState),
			agentPrevScreen: map[string]string{"c1": "hello world"},
			agentLastChange: make(map[string]time.Time),
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("XXXXX world"),
		}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking", m.agentStates["c1"])
		}
	})

	t.Run("screen changed below threshold stays waiting", func(t *testing.T) {
		m := model{
			agentStates:     make(map[string]agentState),
			agentPrevScreen: map[string]string{"c1": "hello world"},
			agentLastChange: map[string]time.Time{"c1": now.Add(-31 * time.Second)},
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("hellX world"), // 1 char diff < threshold
		}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", m.agentStates["c1"])
		}
	})

	t.Run("no change but recent activity stays working", func(t *testing.T) {
		m := model{
			agentStates:     make(map[string]agentState),
			agentPrevScreen: map[string]string{"c1": "hello world"},
			agentLastChange: map[string]time.Time{"c1": now.Add(-10 * time.Second)},
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("hello world"), // no change, but last change was 10s ago
		}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking", m.agentStates["c1"])
		}
	})

	t.Run("no change and stale activity means waiting", func(t *testing.T) {
		m := model{
			agentStates:     make(map[string]agentState),
			agentPrevScreen: map[string]string{"c1": "hello world"},
			agentLastChange: map[string]time.Time{"c1": now.Add(-31 * time.Second)},
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("hello world"),
		}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", m.agentStates["c1"])
		}
	})

	t.Run("no tmux session means not running", func(t *testing.T) {
		m := model{
			agentStates:     map[string]agentState{"c1": agentWorking},
			agentPrevScreen: make(map[string]string),
			agentLastChange: make(map[string]time.Time),
		}
		// OK=true but this won't happen — a missing capture (not in map) means no session
		// Let's test with an OK=false entry explicitly
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": {Content: "", OK: false},
		}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentNotRunning {
			t.Fatalf("got %d, want agentNotRunning", m.agentStates["c1"])
		}
	})

	t.Run("first capture assumes waiting", func(t *testing.T) {
		m := model{
			agentStates:     make(map[string]agentState),
			agentPrevScreen: make(map[string]string),
			agentLastChange: make(map[string]time.Time),
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("spinner content here"),
		}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentWaiting {
			t.Fatalf("got %d, want agentWaiting on first capture", m.agentStates["c1"])
		}
	})

	t.Run("detects tool from screen content", func(t *testing.T) {
		m := model{
			agentStates:     make(map[string]agentState),
			agentTools:      make(map[string]state.AgentTool),
			agentPrevScreen: map[string]string{"c1": "old content"},
			agentLastChange: make(map[string]time.Time),
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("codex> reading files..."),
		}, []string{"c1"}, now)
		if m.agentTools["c1"] != state.AgentToolCodex {
			t.Fatalf("got %q, want %q", m.agentTools["c1"], state.AgentToolCodex)
		}
	})

	t.Run("preserves tool on capture failure", func(t *testing.T) {
		m := model{
			agentStates:     map[string]agentState{"c1": agentWorking},
			agentTools:      map[string]state.AgentTool{"c1": state.AgentToolClaude},
			agentPrevScreen: map[string]string{"c1": "prev"},
			agentLastChange: map[string]time.Time{"c1": now},
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{}, []string{"c1"}, now)
		if m.agentTools["c1"] != state.AgentToolClaude {
			t.Fatalf("tool not preserved: got %q", m.agentTools["c1"])
		}
	})

	t.Run("preserves state on capture failure", func(t *testing.T) {
		m := model{
			agentStates:     map[string]agentState{"c1": agentWorking},
			agentPrevScreen: map[string]string{"c1": "prev"},
			agentLastChange: map[string]time.Time{"c1": now},
		}
		// c1 in expectedIDs but missing from captures (docker exec failure)
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{}, []string{"c1"}, now)
		if m.agentStates["c1"] != agentWorking {
			t.Fatalf("got %d, want agentWorking (preserved)", m.agentStates["c1"])
		}
	})

	t.Run("cleans up removed containers", func(t *testing.T) {
		m := model{
			agentStates:     map[string]agentState{"c1": agentWorking, "c2": agentWaiting},
			agentPrevScreen: map[string]string{"c1": "a", "c2": "b"},
			agentLastChange: map[string]time.Time{"c1": now, "c2": now},
		}
		m.deriveAgentStates(map[string]devcontainer.ScreenCapture{
			"c1": capture("a"),
		}, []string{"c1"}, now)
		if _, ok := m.agentStates["c2"]; ok {
			t.Fatal("c2 should have been cleaned up")
		}
		if _, ok := m.agentPrevScreen["c2"]; ok {
			t.Fatal("c2 prev screen should have been cleaned up")
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
