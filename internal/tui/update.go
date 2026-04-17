package tui

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/version"
	tea "github.com/charmbracelet/bubbletea"
)

// updateCheckMsg is sent when the background update check completes.
type updateCheckMsg struct {
	latestVersion string // empty if no update or error
}

// updateInstalledMsg is sent after the install script finishes.
//
// On success the top-level model stores path/args and quits the TUI;
// Run() then replaces the current process with the new binary via
// syscall.Exec. This is why the install script itself must NOT exec
// the new binary: doing so would leave the new fleet running as a
// grandchild of the old fleet (old-fleet → tea.ExecProcess shell →
// exec'd new fleet), so ^C'ing the new fleet would drop the user back
// into the old fleet. syscall.Exec from the Go process replaces the
// old fleet outright — no nesting.
type updateInstalledMsg struct {
	err  error
	path string   // absolute path to the new binary
	args []string // full argv for the replacement process (argv[0] == path)
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

// performUpdateCmd runs the install script via tea.ExecProcess and, on
// success, signals Run() (via updateInstalledMsg) to quit the TUI and
// exec-replace this process with the new fleet binary. If inHostTmux
// is true the --tmux flag is preserved on the replacement argv.
func performUpdateCmd(inHostTmux bool) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		self = "fleet"
	}

	args := []string{self}
	if inHostTmux {
		args = append(args, "--tmux")
	}

	return tea.ExecProcess(
		updateShellCmd(),
		func(err error) tea.Msg {
			return updateInstalledMsg{err: err, path: self, args: args}
		},
	)
}

// updateShellCmd builds the shell command that downloads and installs
// the latest fleet binary via install.sh. It intentionally stops after
// install — the caller replaces the current process with the new
// binary (see the updateInstalledMsg doc comment for why).
func updateShellCmd() *exec.Cmd {
	script := `curl -sL https://raw.githubusercontent.com/BenjaminBenetti/fleet-man/main/install.sh | sh`
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}
