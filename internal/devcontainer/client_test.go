package devcontainer

import "testing"

func TestParseProbeOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantTool  string
		wantState string
		wantTicks int64
		wantFound bool
	}{
		{"claude working", "claude working\n", "claude", "working", 0, true},
		{"claude waiting", "claude waiting\n", "claude", "waiting", 0, true},
		{"copilot working", "copilot working\n", "copilot", "working", 0, true},
		{"copilot waiting", "copilot waiting\n", "copilot", "waiting", 0, true},
		{"codex ticks", "codex 12345\n", "codex", "", 12345, true},
		{"gemini ticks", "gemini 0\n", "gemini", "", 0, true},
		{"not found", "- -\n", "", "", 0, false},
		{"empty", "", "", "", 0, false},
		{"whitespace ticks", "  codex  67890  \n", "codex", "", 67890, true},
		{"bad ticks", "codex error\n", "", "", 0, false},
		{"single field", "claude\n", "", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, found := parseProbeOutput(tt.output)
			if found != tt.wantFound || r.Tool != tt.wantTool || r.State != tt.wantState || r.CPUTicks != tt.wantTicks {
				t.Errorf("parseProbeOutput(%q) = (%+v, %v), want ({Tool:%q State:%q CPUTicks:%d}, %v)",
					tt.output, r, found, tt.wantTool, tt.wantState, tt.wantTicks, tt.wantFound)
			}
		})
	}
}
