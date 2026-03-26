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

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "agent-1", "agent-1"},
		{"with dot", "my.instance", "my-instance"},
		{"with colon", "foo:bar", "foo-bar"},
		{"mixed", "a.b:c", "a-b-c"},
		{"empty", "", "fleet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSessionName(tt.in)
			if got != tt.want {
				t.Fatalf("sanitizeSessionName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// --- shellCommand (tmux-wrapped) tests ---

func TestShellCommandProducesTmux(t *testing.T) {
	cfg := state.DefaultConfig()
	got := shellCommand(cfg, "agent-1")
	if len(got) != 3 || got[0] != "sh" || got[1] != "-c" {
		t.Fatalf("shellCommand() = %v, want [sh -c ...]", got)
	}
	script := got[2]
	if !strings.Contains(script, "command -v tmux") {
		t.Errorf("script missing tmux install check: %s", script)
	}
	if !strings.Contains(script, "exec tmux -u new-session -A -s 'agent-1'") {
		t.Errorf("script missing tmux new-session -A: %s", script)
	}
	if !strings.Contains(script, "bind-key -n C-q detach-client") {
		t.Errorf("script missing ctrl+q keybinding: %s", script)
	}
}

func TestShellCommandWithDotfilesAndTmux(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/dots"
	cfg.DotfilesSettings.InstallScript = "install.sh"

	got := shellCommand(cfg, "worker-2")
	script := got[2]

	// Dotfiles setup should come before tmux
	dotIdx := strings.Index(script, "[ ! -d ~/dotfiles ]")
	tmuxIdx := strings.Index(script, "exec tmux")
	if dotIdx < 0 || tmuxIdx < 0 {
		t.Fatalf("script missing dotfiles or tmux: %s", script)
	}
	if dotIdx > tmuxIdx {
		t.Errorf("dotfiles setup should come before tmux: %s", script)
	}
	if !strings.Contains(script, "git clone 'https://github.com/user/dots' ~/dotfiles") {
		t.Errorf("script missing git clone: %s", script)
	}
	if !strings.Contains(script, "sh 'install.sh'") {
		t.Errorf("script missing install script: %s", script)
	}
	if strings.HasSuffix(script, "exec bash") {
		t.Errorf("script should not end with exec bash: %s", script)
	}
}

func TestShellCommandSanitizesSessionName(t *testing.T) {
	cfg := state.DefaultConfig()
	got := shellCommand(cfg, "my.instance:1")
	script := got[2]
	if !strings.Contains(script, "tmux -u new-session -A -s 'my-instance-1'") {
		t.Errorf("script should sanitize session name: %s", script)
	}
}

// --- freshShellCommand tests ---

func TestFreshShellCommandNilConfig(t *testing.T) {
	got := freshShellCommand(nil)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("freshShellCommand(nil) = %v, want [bash]", got)
	}
}

func TestFreshShellCommandEmptyDotfiles(t *testing.T) {
	cfg := state.DefaultConfig()
	got := freshShellCommand(cfg)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("freshShellCommand(empty) = %v, want [bash]", got)
	}
}

func TestFreshShellCommandRepoOnlyNoScript(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/dots"
	got := freshShellCommand(cfg)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("freshShellCommand(repo only) = %v, want [bash]", got)
	}
}

func TestFreshShellCommandScriptOnlyNoRepo(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.InstallScript = "install.sh"
	got := freshShellCommand(cfg)
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("freshShellCommand(script only) = %v, want [bash]", got)
	}
}

func TestFreshShellCommandBothSet(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/dots"
	cfg.DotfilesSettings.InstallScript = "install.sh"

	got := freshShellCommand(cfg)
	if len(got) != 3 {
		t.Fatalf("freshShellCommand() returned %d args, want 3", len(got))
	}
	if got[0] != "sh" || got[1] != "-c" {
		t.Fatalf("freshShellCommand() = %v, want [sh -c ...]", got)
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

func TestFreshShellCommandQuotesSpecialCharacters(t *testing.T) {
	cfg := state.DefaultConfig()
	cfg.DotfilesSettings.RepoURL = "https://github.com/user/it's-dots"
	cfg.DotfilesSettings.InstallScript = "my script.sh"

	got := freshShellCommand(cfg)
	if len(got) != 3 {
		t.Fatalf("freshShellCommand() returned %d args, want 3", len(got))
	}

	script := got[2]
	if !strings.Contains(script, `'https://github.com/user/it'\''s-dots'`) {
		t.Errorf("script has incorrect repo quoting: %s", script)
	}
	if !strings.Contains(script, "'my script.sh'") {
		t.Errorf("script has incorrect script quoting: %s", script)
	}
}
