package backend

import "testing"

func TestParseToolProbeOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantTool string
		wantOK   bool
	}{
		{"claude detected", "claude\n", "claude", true},
		{"copilot detected", "copilot\n", "copilot", true},
		{"codex detected", "codex\n", "codex", true},
		{"gemini detected", "gemini\n", "gemini", true},
		{"no agent", "-\n", "", true},
		{"empty output (exec failure)", "", "", false},
		{"whitespace only (exec failure)", "  \n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := ParseToolProbeOutput(tt.output)
			if ok != tt.wantOK {
				t.Fatalf("ParseToolProbeOutput(%q) ok = %v, want %v", tt.output, ok, tt.wantOK)
			}
			if tool != tt.wantTool {
				t.Fatalf("ParseToolProbeOutput(%q) tool = %q, want %q", tt.output, tool, tt.wantTool)
			}
		})
	}
}

func TestParseAllSessionsOutput(t *testing.T) {
	mk := func(name, content string) string {
		return sessionMarker + name + "\n" + content
	}

	t.Run("empty input means no sessions", func(t *testing.T) {
		got := ParseAllSessionsOutput("")
		if len(got) != 0 {
			t.Fatalf("expected 0 sessions, got %d", len(got))
		}
	})

	t.Run("single session with content", func(t *testing.T) {
		input := mk("main", "line1\nline2\n")
		got := ParseAllSessionsOutput(input)
		if len(got) != 1 {
			t.Fatalf("expected 1 session, got %d", len(got))
		}
		sc, ok := got["main"]
		if !ok {
			t.Fatalf("session 'main' missing")
		}
		if !sc.OK {
			t.Fatal("expected OK=true")
		}
		if sc.Content != "line1\nline2" {
			t.Fatalf("got content %q", sc.Content)
		}
	})

	t.Run("multiple sessions are demultiplexed", func(t *testing.T) {
		input := mk("main", "alpha\n") + mk("session-2", "beta\nbeta2\n") + mk("hex-abc", "")
		got := ParseAllSessionsOutput(input)
		if len(got) != 3 {
			t.Fatalf("expected 3 sessions, got %d", len(got))
		}
		if got["main"].Content != "alpha" {
			t.Fatalf("main content: %q", got["main"].Content)
		}
		if got["session-2"].Content != "beta\nbeta2" {
			t.Fatalf("session-2 content: %q", got["session-2"].Content)
		}
		if got["hex-abc"].Content != "" {
			t.Fatalf("hex-abc should be empty, got %q", got["hex-abc"].Content)
		}
	})

	t.Run("session names with special characters survive", func(t *testing.T) {
		// Group sessions use ~ as separator; any printable ASCII
		// other than RS (\x1e) must round-trip through the marker.
		input := mk("fleet~abc123~def", "content")
		got := ParseAllSessionsOutput(input)
		sc, ok := got["fleet~abc123~def"]
		if !ok {
			t.Fatalf("expected fleet~abc123~def in result; got keys: %v", keys(got))
		}
		if sc.Content != "content" {
			t.Fatalf("got content %q", sc.Content)
		}
	})

	t.Run("output without markers is ignored", func(t *testing.T) {
		// tmux server not running → script exits with no output
		// before any marker — nothing to record.
		got := ParseAllSessionsOutput("orphan content with no marker\n")
		if len(got) != 0 {
			t.Fatalf("expected 0 sessions, got %d", len(got))
		}
	})
}

func keys(m map[string]ScreenCapture) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
