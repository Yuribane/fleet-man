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

// shellCommand returns the command to run inside a devcontainer.
// When both dotfiles settings are configured, it returns a sh -c script
// that clones the repo to ~/dotfiles and runs the install script before
// launching bash. If ~/dotfiles already exists, it skips straight to bash.
// When dotfiles are not configured, it returns just ["bash"].
func shellCommand(cfg *state.Config) []string {
	if cfg == nil {
		return []string{"bash"}
	}

	repo := cfg.DotfilesSettings.RepoURL
	script := cfg.DotfilesSettings.InstallScript
	if repo == "" || script == "" {
		return []string{"bash"}
	}

	inner := fmt.Sprintf(
		`if [ ! -d ~/dotfiles ]; then echo '==> Cloning dotfiles...'; git clone %s ~/dotfiles && cd ~/dotfiles && sh %s; fi; exec bash`,
		shQuote(repo), shQuote(script),
	)
	return []string{"sh", "-c", inner}
}
