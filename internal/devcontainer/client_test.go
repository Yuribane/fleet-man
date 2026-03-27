package devcontainer

import "testing"

func TestBinaryMatchPattern(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude", `(^|\/)claude$`},
		{"codex", `(^|\/)codex$`},
		{"gemini", `(^|\/)gemini$`},
		{"copilot", `(^|\/)copilot$`},
		{"", ""},
		{"a", `(^|\/)a$`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := binaryMatchPattern(tt.input)
			if got != tt.want {
				t.Errorf("binaryMatchPattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseProbeOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantTool  string
		wantTicks int64
		wantFound bool
	}{
		{"claude found", "claude 12345\n", "claude", 12345, true},
		{"copilot found", "copilot 0\n", "copilot", 0, true},
		{"not found marker", "- -1\n", "", 0, false},
		{"empty", "", "", 0, false},
		{"whitespace", "  codex  67890  \n", "codex", 67890, true},
		{"bad ticks", "claude error\n", "", 0, false},
		{"single field", "claude\n", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, found := parseProbeOutput(tt.output)
			if found != tt.wantFound || r.Tool != tt.wantTool || r.CPUTicks != tt.wantTicks {
				t.Errorf("parseProbeOutput(%q) = (%+v, %v), want ({Tool:%q CPUTicks:%d}, %v)",
					tt.output, r, found, tt.wantTool, tt.wantTicks, tt.wantFound)
			}
		})
	}
}
