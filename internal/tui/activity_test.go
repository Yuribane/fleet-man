package tui

import (
	"testing"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

// allFor is a test helper that wraps a single sessionName→content
// mapping into a backend.AllSessions with OK=true.
func allFor(sessions map[string]string) backend.AllSessions {
	out := backend.AllSessions{Sessions: make(map[string]backend.ScreenCapture, len(sessions)), OK: true}
	for name, content := range sessions {
		out.Sessions[name] = backend.ScreenCapture{Content: content, OK: true}
	}
	return out
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
		tr.prevScreen["c1"] = map[string]string{"main": "hello world"}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "XXXXX world"}),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking", tr.State("c1"))
		}
	})

	t.Run("screen changed below threshold stays waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = map[string]string{"main": "hello world"}
		tr.lastChange["c1"] = map[string]time.Time{"main": now.Add(-31 * time.Second)}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "hellX world"}), // 1 char diff < threshold
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", tr.State("c1"))
		}
	})

	t.Run("no change but recent activity stays working", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = map[string]string{"main": "hello world"}
		tr.lastChange["c1"] = map[string]time.Time{"main": now.Add(-10 * time.Second)}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "hello world"}),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking", tr.State("c1"))
		}
	})

	t.Run("no change and stale activity means waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = map[string]string{"main": "hello world"}
		tr.lastChange["c1"] = map[string]time.Time{"main": now.Add(-31 * time.Second)}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "hello world"}),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", tr.State("c1"))
		}
	})

	t.Run("exec failure marks not running", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.Update(map[string]backend.AllSessions{
			"c1": {OK: false},
		}, nil, []string{"c1"}, now)
		// OK=false is treated as transient — preserve previous state
		// (which is zero/agentNotRunning here since tracker is fresh)
		if tr.State("c1") != agentNotRunning {
			t.Fatalf("got %d, want agentNotRunning", tr.State("c1"))
		}
	})

	t.Run("ok but no sessions means not running", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.Update(map[string]backend.AllSessions{
			"c1": {Sessions: map[string]backend.ScreenCapture{}, OK: true},
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentNotRunning {
			t.Fatalf("got %d, want agentNotRunning", tr.State("c1"))
		}
	})

	t.Run("first capture assumes waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "spinner content here"}),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting on first capture", tr.State("c1"))
		}
	})

	t.Run("activity in any session marks container working", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = map[string]string{
			"main":      "static content",
			"session-2": "old agent content",
		}
		// session-2 has a big change (agent active there); main is unchanged.
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{
				"main":      "static content",
				"session-2": "NEW agent content",
			}),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking (session-2 changed)", tr.State("c1"))
		}
	})

	t.Run("idle in all sessions means waiting", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = map[string]string{
			"main":      "static content",
			"session-2": "another static",
		}
		tr.lastChange["c1"] = map[string]time.Time{
			"main":      now.Add(-30 * time.Second),
			"session-2": now.Add(-30 * time.Second),
		}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{
				"main":      "static content",
				"session-2": "another static",
			}),
		}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWaiting {
			t.Fatalf("got %d, want agentWaiting", tr.State("c1"))
		}
	})

	t.Run("detects tool from probe results", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.prevScreen["c1"] = map[string]string{"main": "old content"}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "some screen content"}),
		}, map[string]string{"c1": "codex"}, []string{"c1"}, now)
		if tr.Tool("c1") != state.AgentToolCodex {
			t.Fatalf("got %q, want %q", tr.Tool("c1"), state.AgentToolCodex)
		}
	})

	t.Run("tool detected even when no sessions captured", func(t *testing.T) {
		// Regression: in the single-session implementation the
		// !sc.OK branch short-circuited tool detection. The probe
		// must record claude regardless of session state so the UI
		// shows the right label.
		tr := NewActivityTracker()
		tr.Update(map[string]backend.AllSessions{
			"c1": {Sessions: map[string]backend.ScreenCapture{}, OK: true},
		}, map[string]string{"c1": "claude"}, []string{"c1"}, now)
		if tr.Tool("c1") != state.AgentToolClaude {
			t.Fatalf("got %q, want %q", tr.Tool("c1"), state.AgentToolClaude)
		}
	})

	t.Run("clears tool and marks idle when probe finds no agent", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.tools["c1"] = state.AgentToolClaude
		tr.states["c1"] = agentWaiting
		tr.prevScreen["c1"] = map[string]string{"main": "old content"}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "some screen content"}),
		}, map[string]string{"c1": ""}, []string{"c1"}, now)
		if tr.Tool("c1") != "" {
			t.Fatalf("tool should be cleared, got %q", tr.Tool("c1"))
		}
		if tr.State("c1") != agentNotRunning {
			t.Fatalf("got %d, want agentNotRunning", tr.State("c1"))
		}
	})

	t.Run("preserves tool on capture failure", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.tools["c1"] = state.AgentToolClaude
		tr.prevScreen["c1"] = map[string]string{"main": "prev"}
		tr.lastChange["c1"] = map[string]time.Time{"main": now}
		tr.Update(map[string]backend.AllSessions{}, nil, []string{"c1"}, now)
		if tr.Tool("c1") != state.AgentToolClaude {
			t.Fatalf("tool not preserved: got %q", tr.Tool("c1"))
		}
	})

	t.Run("preserves state on capture failure", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.prevScreen["c1"] = map[string]string{"main": "prev"}
		tr.lastChange["c1"] = map[string]time.Time{"main": now}
		tr.Update(map[string]backend.AllSessions{}, nil, []string{"c1"}, now)
		if tr.State("c1") != agentWorking {
			t.Fatalf("got %d, want agentWorking (preserved)", tr.State("c1"))
		}
	})

	t.Run("cleans up removed containers", func(t *testing.T) {
		tr := NewActivityTracker()
		tr.states["c1"] = agentWorking
		tr.states["c2"] = agentWaiting
		tr.prevScreen["c1"] = map[string]string{"main": "a"}
		tr.prevScreen["c2"] = map[string]string{"main": "b"}
		tr.Update(map[string]backend.AllSessions{
			"c1": allFor(map[string]string{"main": "a"}),
		}, nil, []string{"c1"}, now)
		if tr.State("c2") != agentNotRunning {
			t.Fatal("c2 should have been cleaned up")
		}
	})
}
