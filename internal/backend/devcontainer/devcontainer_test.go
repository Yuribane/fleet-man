package devcontainer

import (
	"slices"
	"testing"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
)

func TestScreenCaptureZeroValue(t *testing.T) {
	// A zero-value ScreenCapture should indicate failure (OK=false).
	var sc backend.ScreenCapture
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
			tool, ok := backend.ParseToolProbeOutput(tt.output)
			if ok != tt.wantOK {
				t.Fatalf("ParseToolProbeOutput(%q) ok = %v, want %v", tt.output, ok, tt.wantOK)
			}
			if tool != tt.wantTool {
				t.Fatalf("ParseToolProbeOutput(%q) tool = %q, want %q", tt.output, tool, tt.wantTool)
			}
		})
	}
}

func TestDevcontainerEnvBuildKitMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantEnv string
		wantErr bool
	}{
		{name: "unset delegates to default"},
		{name: "auto delegates to default", mode: "auto"},
		{name: "never disables buildkit", mode: "never", wantEnv: "DOCKER_BUILDKIT=0"},
		{name: "invalid errors", mode: "sometimes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FLEET_DEVCONTAINER_BUILDKIT", tt.mode)
			env, err := devcontainerEnv([]string{"PATH=/bin"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("devcontainerEnv() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("devcontainerEnv() error = %v", err)
			}
			if tt.wantEnv != "" && !slices.Contains(env, tt.wantEnv) {
				t.Fatalf("devcontainerEnv() missing %q in %#v", tt.wantEnv, env)
			}
			if tt.wantEnv == "" && slices.Contains(env, "DOCKER_BUILDKIT=0") {
				t.Fatalf("devcontainerEnv() unexpectedly disabled BuildKit: %#v", env)
			}
		})
	}
}
