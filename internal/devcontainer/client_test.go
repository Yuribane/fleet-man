package devcontainer

import "testing"

func TestScreenCaptureZeroValue(t *testing.T) {
	// A zero-value ScreenCapture should indicate failure (OK=false).
	var sc ScreenCapture
	if sc.OK {
		t.Fatal("zero-value ScreenCapture should have OK=false")
	}
	if sc.Content != "" {
		t.Fatal("zero-value ScreenCapture should have empty Content")
	}
}

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
		{"empty output", "", "", false},
		{"whitespace only", "  \n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := parseToolProbeOutput(tt.output)
			if ok != tt.wantOK {
				t.Fatalf("parseToolProbeOutput(%q) ok = %v, want %v", tt.output, ok, tt.wantOK)
			}
			if tool != tt.wantTool {
				t.Fatalf("parseToolProbeOutput(%q) tool = %q, want %q", tt.output, tool, tt.wantTool)
			}
		})
	}
}
