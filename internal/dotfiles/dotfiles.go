package dotfiles

import (
	"fmt"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

// ShQuote returns s wrapped in single quotes with any embedded
// single quotes escaped using the '\'' idiom.
func ShQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// SetupScript returns the raw shell snippet for dotfiles installation
// regardless of the auto-install setting. Returns empty if dotfiles are not
// configured (repo URL or install script missing).
func SetupScript(cfg *state.Config) string {
	if cfg == nil {
		return ""
	}
	repo := cfg.DotfilesSettings.RepoURL
	script := cfg.DotfilesSettings.InstallScript
	if repo == "" || script == "" {
		return ""
	}
	// Run the install script under setsid so that any long-lived
	// processes it spawns (e.g. sshfs's ssh subprocess) are placed in a
	// new session/process group.  Without this, those children inherit
	// the docker-exec pty's process group and receive SIGHUP when the
	// user detaches from tmux, killing SSHFS mounts.
	return fmt.Sprintf(
		`if [ ! -d ~/dotfiles ]; then echo '==> Cloning dotfiles...'; GIT_SSH_COMMAND='ssh -o StrictHostKeyChecking=accept-new' git clone %s ~/dotfiles && (cd ~/dotfiles && setsid sh %s); fi; `,
		ShQuote(repo), ShQuote(script),
	)
}
