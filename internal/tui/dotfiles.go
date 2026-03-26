package tui

import (
	"fmt"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

// shQuote returns s wrapped in single quotes with any embedded
// single quotes escaped using the '\'' idiom.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// sanitizeSessionName replaces characters that are problematic in
// socket filenames with hyphens.
func sanitizeSessionName(name string) string {
	r := strings.NewReplacer(".", "-", ":", "-", "/", "-")
	s := r.Replace(name)
	if s == "" {
		return "fleet"
	}
	return s
}

// dotfilesSetup returns a shell snippet that clones and installs dotfiles,
// or an empty string if dotfiles are not configured.
func dotfilesSetup(cfg *state.Config) string {
	if cfg == nil {
		return ""
	}
	repo := cfg.DotfilesSettings.RepoURL
	script := cfg.DotfilesSettings.InstallScript
	if repo == "" || script == "" {
		return ""
	}
	return fmt.Sprintf(
		`if [ ! -d ~/dotfiles ]; then echo '==> Cloning dotfiles...'; git clone %s ~/dotfiles && cd ~/dotfiles && sh %s; fi; `,
		shQuote(repo), shQuote(script),
	)
}

// shellCommand returns the command to run inside a devcontainer with a
// persistent tmux session. The session is named after the instance so
// that reconnecting reattaches to the running session.
// If tmux is not installed in the container it is auto-installed.
//
// tmux new-session -A handles all cases in one command:
//   - No session exists: creates a new one.
//   - Session exists: attaches to it.
//
// Ctrl+Q detaches and keeps processes running.
func shellCommand(cfg *state.Config, instanceName string) []string {
	setup := dotfilesSetup(cfg)
	session := sanitizeSessionName(instanceName)
	tmuxInstall := `command -v tmux >/dev/null 2>&1 || { echo '==> Installing tmux...'; (sudo apt-get update -qq && sudo apt-get install -y -qq tmux) 2>/dev/null || (sudo apk add tmux) 2>/dev/null || (sudo dnf install -y tmux) 2>/dev/null; }; `
	inner := setup + tmuxInstall + fmt.Sprintf(
		`exec tmux -u new-session -A -s %s \; bind-key -n C-q detach-client \; set status-right ' ctrl+q: detach '`,
		shQuote(session),
	)
	return []string{"sh", "-c", inner}
}

// freshShellCommand returns the command to run inside a devcontainer
// without tmux. Used by the "open in new terminal" action where a fresh,
// non-persistent session is desired.
func freshShellCommand(cfg *state.Config) []string {
	setup := dotfilesSetup(cfg)
	if setup == "" {
		return []string{"bash"}
	}
	return []string{"sh", "-c", setup + "exec bash"}
}
