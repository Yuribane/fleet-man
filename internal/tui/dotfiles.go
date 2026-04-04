package tui

import (
	"fmt"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/dotfiles"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

// shQuote returns s wrapped in single quotes with any embedded
// single quotes escaped using the '\'' idiom.
func shQuote(s string) string {
	return dotfiles.ShQuote(s)
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

// dotfilesSetupScript returns the raw shell snippet for dotfiles installation
// regardless of the auto-install setting. Returns empty if dotfiles are not
// configured (repo URL or install script missing).
func dotfilesSetupScript(cfg *state.Config) string {
	return dotfiles.SetupScript(cfg)
}

// dotfilesSetup returns a shell snippet that clones and installs dotfiles,
// or an empty string if dotfiles are not configured or auto-install is enabled
// (in which case dotfiles are installed in the background on instance creation).
func dotfilesSetup(cfg *state.Config) string {
	if cfg != nil && cfg.DotfilesSettings.AutoInstall {
		return ""
	}
	return dotfilesSetupScript(cfg)
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
// Ctrl+Q or Ctrl+O detaches and keeps processes running.
//
// cols/rows are the caller's terminal dimensions. When non-zero, stty
// is used to correct the remote PTY size before tmux starts. This is
// needed for backends like coder ssh that may report incorrect sizes
// (e.g. 128x128).
func shellCommand(cfg *state.Config, instanceName string, cols, rows int, nested bool) []string {
	setup := dotfilesSetup(cfg)
	session := sanitizeSessionName(instanceName)
	tmuxInstall := `command -v tmux >/dev/null 2>&1 || { echo '==> Installing tmux...'; (sudo apt-get update -qq && sudo apt-get install -y -qq tmux) 2>/dev/null || (sudo apk add tmux) 2>/dev/null || (sudo dnf install -y tmux) 2>/dev/null; }; `
	// coder ssh may report incorrect terminal dimensions (e.g. 128x128).
	// We fix the PTY size with stty before tmux starts, pass -x/-y for
	// new session creation, and set a client-attached hook to force
	// the correct size on every reattach. The hook uses aggressive-resize
	// and resize-window to override tmux's stale client dimensions.
	sizefix := ""
	tmuxSize := ""
	resizeHook := ""
	if cols > 0 && rows > 0 {
		sizefix = fmt.Sprintf(`stty cols %d rows %d 2>/dev/null; `, cols, rows)
		tmuxSize = fmt.Sprintf(` -x %d -y %d`, cols, rows-1)
		resizeHook = fmt.Sprintf(
			` \; set -g aggressive-resize on \; set-hook -g client-attached 'resize-window -x %d -y %d'`,
			cols, rows-1,
		)
	}
	// When nested inside a host tmux (split pane mode), use Ctrl+A as
	// the inner prefix so it doesn't conflict with the outer Ctrl+B.
	prefixConf := ""
	statusRight := ` ctrl+q/ctrl+o: detach `
	if nested {
		prefixConf = ` \; set -g prefix C-x \; bind-key C-x send-prefix`
		statusRight = ` prefix: ctrl+x | ctrl+q/ctrl+o: detach `
	}
	inner := setup + tmuxInstall + sizefix + fmt.Sprintf(
		`exec tmux -u new-session -A -s %s`+tmuxSize+` \; set -g mouse on \; bind-key -n C-q detach-client \; bind-key -n C-o detach-client \; set status-right '%s'`+prefixConf+resizeHook,
		shQuote(session), statusRight,
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
