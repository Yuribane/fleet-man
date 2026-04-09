package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/version"
	tea "github.com/charmbracelet/bubbletea"
)

// updateCheckMsg is sent when the background update check completes.
type updateCheckMsg struct {
	latestVersion string // empty if no update or error
}

// checkUpdateCmd queries the GitHub API for the latest release of fleet-man
// and compares it against the compiled-in version. Returns an updateCheckMsg
// with the new version tag if an update is available.
func checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		current := version.Version
		if current == "" {
			// Dev build — skip update check.
			return updateCheckMsg{}
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("https://api.github.com/repos/BenjaminBenetti/fleet-man/releases/latest")
		if err != nil {
			return updateCheckMsg{}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return updateCheckMsg{}
		}

		var release struct {
			TagName string `json:"tag_name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return updateCheckMsg{}
		}

		if release.TagName != "" && release.TagName != current {
			return updateCheckMsg{latestVersion: release.TagName}
		}

		return updateCheckMsg{}
	}
}

// performUpdateCmd runs the install script and then re-execs fleet.
// If inHostTmux is true the --tmux flag is preserved on relaunch.
func performUpdateCmd(inHostTmux bool) tea.Cmd {
	return tea.ExecProcess(
		updateShellCmd(inHostTmux),
		func(err error) tea.Msg { return execDoneMsg{err} },
	)
}

// updateShellCmd builds the shell command that downloads the latest
// binary via install.sh, then re-execs fleet (preserving --tmux).
func updateShellCmd(inHostTmux bool) *exec.Cmd {
	self, err := os.Executable()
	if err != nil {
		self = "fleet"
	}

	relaunch := self
	if inHostTmux {
		relaunch = fmt.Sprintf("%s --tmux", self)
	}

	script := fmt.Sprintf(
		`curl -sL https://raw.githubusercontent.com/BenjaminBenetti/fleet-man/main/install.sh | sh && exec %s`,
		relaunch,
	)
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	return cmd
}
