package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/portforward"
	tea "github.com/charmbracelet/bubbletea"
)

// ===========================================
// Constants
// ===========================================

// browserProxyPort is the port privoxy listens on inside the container.
const browserProxyPort = 58888

// ===========================================
// Messages
// ===========================================

// browserProxyMsg is sent when the browser proxy setup completes.
type browserProxyMsg struct {
	instanceKey string
	localPort   int
	err         error
}

// ===========================================
// Proxy Setup Script
// ===========================================

// privoxy is used instead of tinyproxy because tinyproxy mishandles
// responses with multiple Set-Cookie headers (common in older auth
// flows that split a large cookie across several lines). privoxy
// forwards them verbatim.

// privoxyEnsureInstalled installs privoxy if it is not already
// present. Mirrors the pattern used by tmuxEnsureInstalled in
// dotfiles.go: try without sudo first, then fall back to sudo,
// across multiple package managers.
var privoxyEnsureInstalled = `command -v privoxy >/dev/null 2>&1 || { echo '==> Installing privoxy...'; (apt-get update -qq && apt-get install -y -qq privoxy) 2>/dev/null || (sudo apt-get update -qq && sudo apt-get install -y -qq privoxy) 2>/dev/null || (apk add privoxy) 2>/dev/null || (sudo apk add privoxy) 2>/dev/null || (dnf install -y privoxy) 2>/dev/null || (sudo dnf install -y privoxy) 2>/dev/null || (yum install -y privoxy) 2>/dev/null || (sudo yum install -y privoxy) 2>/dev/null || { echo 'INSTALL_FAILED'; exit 1; }; }; `

// privoxyAction neutralizes privoxy's built-in privacy/filtering
// behaviors. fleet-man only wants transparent forwarding to dev
// services inside the container — no ad-blocking, no header
// rewriting, no cookie crunching. limit-connect is widened to the
// full port range so HTTPS CONNECT works for services on any port
// (privoxy's default only permits CONNECT to 443).
var privoxyAction = `{ -block -filter -handle-as-image -set-image-blocker -hide-from-header -hide-referrer -hide-user-agent -crunch-incoming-cookies -crunch-outgoing-cookies -session-cookies-only -change-x-forwarded-for +limit-connect{1-65535} }\n/\n`

// privoxyConfig is a minimal privoxy configuration. Allows connections
// from any source (traffic arrives via the port forward, not the open
// network) and loads only fleet-man's own action file so system-wide
// privoxy rules don't leak in. confdir points at the package-installed
// template directory so privoxy can render its own error pages.
var privoxyConfig = `listen-address 0.0.0.0:58888\npermit-access 0.0.0.0/0\ntoggle 0\nenable-remote-toggle 0\nenable-remote-http-toggle 0\nenable-edit-actions 0\nconfdir /etc/privoxy\nactionsfile /tmp/fleet-proxy.action\n`

// proxySetupScript is the single-line shell command that installs
// privoxy (if missing) and starts it idempotently.
//
//	Flow:
//	  1. Install privoxy via the first available package manager
//	     (with sudo fallback).
//	  2. If a privoxy process is already running, exit early.
//	  3. Write action + config files to /tmp/ via printf.
//	  4. Start privoxy as a daemon with an explicit pidfile.
var proxySetupScript = privoxyEnsureInstalled +
	`if [ -f /tmp/fleet-proxy.pid ] && kill -0 $(cat /tmp/fleet-proxy.pid) 2>/dev/null; then echo 'ALREADY_RUNNING'; exit 0; fi; ` +
	`printf '` + privoxyAction + `' > /tmp/fleet-proxy.action; ` +
	`printf '` + privoxyConfig + `' > /tmp/fleet-proxy.conf; ` +
	`privoxy --pidfile /tmp/fleet-proxy.pid /tmp/fleet-proxy.conf && echo 'STARTED' || { echo 'START_FAILED'; exit 1; }`

// ===========================================
// Commands
// ===========================================

// openBrowserProxyCmd returns a tea.Cmd that ensures a browser proxy is
// running for the given instance and then launches a Chromium-based
// browser configured to route traffic through it.
//
//	Sequence:
//	  +-----------------------+
//	  | Existing proxy fwd?   |--yes--> Launch browser
//	  +-----------------------+
//	            |no
//	  +-----------------------+
//	  | Create port forward   |
//	  | (auto local port)     |
//	  +-----------------------+
//	            |
//	  +-----------------------+
//	  | Install / start       |
//	  | privoxy in container  |
//	  +-----------------------+
//	            |
//	  +-----------------------+
//	  | Launch browser with   |
//	  | --proxy-server flag   |
//	  +-----------------------+
func openBrowserProxyCmd(
	pf *portforward.Manager,
	b backend.Backend,
	inst *fleet.Instance,
	instanceKey string,
) tea.Cmd {
	// Capture values for the goroutine.
	workspaceDir := inst.WorkspaceDir
	containerID := inst.ContainerID
	instanceName := inst.Name

	return func() tea.Msg {
		// 1. Reuse an existing browser proxy forward if one exists.
		localPort, found := pf.FindBrowserProxy(instanceKey)

		if !found {
			// 2. Create a new port forward from an auto-selected local
			//    port to privoxy inside the container.
			var err error
			localPort, err = pf.AddBrowserProxy(
				instanceKey,
				browserProxyPort,
				b.PortForwardCommand,
				containerID,
				b.ResolveHostname,
			)
			if err != nil {
				return browserProxyMsg{instanceKey: instanceKey, err: fmt.Errorf("port forward: %w", err)}
			}
		}

		// 3. Ensure privoxy is installed and running inside the container.
		if err := ensureProxyRunning(b, workspaceDir); err != nil {
			return browserProxyMsg{instanceKey: instanceKey, err: err}
		}

		// 4. Launch the browser.
		if err := launchBrowser(localPort, instanceName); err != nil {
			return browserProxyMsg{instanceKey: instanceKey, err: fmt.Errorf("launch browser: %w", err)}
		}

		return browserProxyMsg{instanceKey: instanceKey, localPort: localPort}
	}
}

// ===========================================
// Helpers
// ===========================================

// ensureProxyRunning shells into the container and runs the proxy setup
// script. It returns an error if privoxy cannot be installed or started.
func ensureProxyRunning(b backend.Backend, workspaceDir string) error {
	cmd := b.ExecCommand(workspaceDir, []string{"sh", "-c", proxySetupScript})
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("proxy setup: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	result := strings.TrimSpace(string(out))
	// The script echoes a status keyword as its last line of output.
	// Extract it in case earlier lines contain install chatter.
	lines := strings.Split(result, "\n")
	status := lines[len(lines)-1]

	switch {
	case strings.Contains(status, "INSTALL_FAILED"):
		return fmt.Errorf("could not install privoxy (no supported package manager found)")
	case strings.Contains(status, "START_FAILED"):
		return fmt.Errorf("privoxy failed to start — check container logs")
	case strings.Contains(status, "STARTED"):
		// Give the daemon a moment to bind to the port.
		time.Sleep(500 * time.Millisecond)
	}
	// "ALREADY_RUNNING" falls through — nothing extra to do.
	return nil
}

// launchBrowser opens a Chromium-based browser with its HTTP/HTTPS/WS/WSS
// traffic routed through the proxy on localPort. A per-instance user data
// directory avoids profile conflicts when multiple browsers are open for
// different containers.
func launchBrowser(localPort int, instanceName string) error {
	proxyArg := fmt.Sprintf("--proxy-server=http://localhost:%d", localPort)
	dataDir := fmt.Sprintf("/tmp/fleet-browser-%s", instanceName)

	browsers := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium-browser",
		"chromium",
	}

	for _, browser := range browsers {
		if _, err := exec.LookPath(browser); err == nil {
			cmd := exec.Command(browser,
				proxyArg,
				// By default Chrome bypasses the proxy for loopback
				// addresses. The whole point of this feature is to
				// reach services on the container's localhost, so we
				// must disable that bypass.
				"--proxy-bypass-list=<-loopback>",
				"--user-data-dir="+dataDir,
				"--no-first-run",
				"--no-default-browser-check",
				"about:blank",
			)
			return cmd.Start()
		}
	}

	return fmt.Errorf("no Chromium-based browser found (tried: %s)", strings.Join(browsers, ", "))
}
