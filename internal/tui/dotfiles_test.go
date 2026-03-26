package tui

import (
	"strings"
	"testing"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

func TestShQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quote", "it's", `'it'\''s'`},
		{"empty", "", "''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shQuote(tt.in)
			if got != tt.want {
				t.Fatalf("shQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShellCommandNilConfig(t *testing.T) {
	got := shellCommand(nil)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("shellCommand(nil) = %v, want [bash]", got)
	}
}

func TestShellCommandEmptyDotfiles(t *testing.T) {
	cfg := state.DefaultConfig()
	got := shellCommand(cfg)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("shellCommand(empty) = %v, want [bash]", got)
	}
}

func TestShellCommandRepoOnlyNoScript(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/dots"
	got := shellCommand(cfg)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("shellCommand(repo only) = %v, want [bash]", got)
	}
}

func TestShellCommandScriptOnlyNoRepo(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.InstallScript = "install.sh"
	got := shellCommand(cfg)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("shellCommand(script only) = %v, want [bash]", got)
	}
}

func TestShellCommandBothSet(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/dots"
	cfg.DotfilesSettings.InstallScript = "install.sh"

	got := shellCommand(cfg)
	if len(got) != 3 {
		t.Fatalf("shellCommand() returned %d args, want 3", len(got))
	}
	if got[0] != "sh" || got[1] != "-c" {
		t.Fatalf("shellCommand() = %v, want [sh -c ...]", got)
	}

	script := got[2]
	if !strings.Contains(script, "[ ! -d ~/dotfiles ]") {
		t.Error("script missing ~/dotfiles existence check")
	}
	if !strings.Contains(script, "git clone 'https://github.com/user/dots' ~/dotfiles") {
		t.Errorf("script missing git clone: %s", script)
	}
	if !strings.Contains(script, "sh 'install.sh'") {
		t.Errorf("script missing install script invocation: %s", script)
	}
	if !strings.HasSuffix(script, "exec bash") {
		t.Errorf("script should end with exec bash: %s", script)
	}
}

func TestShellCommandQuotesSpecialCharacters(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/it's-dots"
	cfg.DotfilesSettings.InstallScript = "my script.sh"

	got := shellCommand(cfg)
	if len(got) != 3 {
		t.Fatalf("shellCommand() returned %d args, want 3", len(got))
	}

	script := got[2]
	if !strings.Contains(script, `'https://github.com/user/it'\''s-dots'`) {
		t.Errorf("script has incorrect repo quoting: %s", script)
	}
	if !strings.Contains(script, "'my script.sh'") {
		t.Errorf("script has incorrect script quoting: %s", script)
	}
}
