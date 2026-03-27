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

func TestParseProbeLine(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantTicks int64
		wantFound bool
	}{
		{"valid ticks", "12345\n", 12345, true},
		{"zero ticks", "0\n", 0, true},
		{"not found", "-1\n", -1, false},
		{"empty", "", -1, false},
		{"whitespace", "  67890  \n", 67890, true},
		{"non-numeric", "error\n", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticks, found := parseProbeLine(tt.output)
			if ticks != tt.wantTicks || found != tt.wantFound {
				t.Errorf("parseProbeLine(%q) = (%d, %v), want (%d, %v)",
					tt.output, ticks, found, tt.wantTicks, tt.wantFound)
			}
		})
	}
}
