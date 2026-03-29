package tui

import (
	"testing"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

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

func TestActivityTrackerUpdate(t *testing.T) {
	now := time.Now()

	t.Run("screen changed above threshold marks working", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = "hello world"
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("XXXXX world"),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking", tr.State("c1"))
		}
	})

	t.Run("screen changed below threshold stays waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = "hello world"
		tr.lastChange["c1"] = now.Add(-31 * time.Second)
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("hellX world"), // 1 char diff < threshold
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", tr.State("c1"))
		}
	})

	t.Run("no change but recent activity stays working", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = "hello world"
		tr.lastChange["c1"] = now.Add(-10 * time.Second)
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("hello world"),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking", tr.State("c1"))
		}
	})

	t.Run("no change and stale activity means waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = "hello world"
		tr.lastChange["c1"] = now.Add(-31 * time.Second)
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("hello world"),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", tr.State("c1"))
		}
	})

	t.Run("no tmux session means not running", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": {Content: "", OK: false},
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentNotRunning {
			t.Fatalf("got %d, want agentNotRunning", tr.State("c1"))
		}
	})

	t.Run("first capture assumes waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("spinner content here"),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting on first capture", tr.State("c1"))
		}
	})

	t.Run("detects tool from probe results", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = "old content"
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("some screen content"),
		}, map[string]string{"c1": "codex"}, []string{"c1"}, now)
		if tr.Tool("c1") != state.AgentToolCodex {
			t.Fatalf("got %q, want %q", tr.Tool("c1"), state.AgentToolCodex)
		}
	})

	t.Run("preserves tool when probe finds no agent", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.tools["c1"] = state.AgentToolClaude
		tr.prevScreen["c1"] = "old content"
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("some screen content"),
		}, map[string]string{"c1": ""}, []string{"c1"}, now)
		if tr.Tool("c1") != state.AgentToolClaude {
			t.Fatalf("got %q, want %q", tr.Tool("c1"), state.AgentToolClaude)
		}
	})

	t.Run("preserves tool on capture failure", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.tools["c1"] = state.AgentToolClaude
		tr.prevScreen["c1"] = "prev"
		tr.lastChange["c1"] = now
		tr.Update(map[string]devcontainer.ScreenCapture{}, nil, []string{"c1"}, now)
		if tr.Tool("c1") != state.AgentToolClaude {
			t.Fatalf("tool not preserved: got %q", tr.Tool("c1"))
		}
	})

	t.Run("preserves state on capture failure", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.prevScreen["c1"] = "prev"
		tr.lastChange["c1"] = now
		tr.Update(map[string]devcontainer.ScreenCapture{}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking (preserved)", tr.State("c1"))
		}
	})

	t.Run("cleans up removed containers", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.states["c2"] = agentWaiting
		tr.prevScreen["c1"] = "a"
		tr.prevScreen["c2"] = "b"
		tr.lastChange["c1"] = now
		tr.lastChange["c2"] = now
		tr.Update(map[string]devcontainer.ScreenCapture{
			"c1": capture("a"),
		}, nil, []string{"c1"}, now)
		if tr.State("c2") != agentNotRunning {
			t.Fatal("c2 should have been cleaned up")
		}
	})
}
