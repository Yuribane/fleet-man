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

// browserProxyPort is the port tinyproxy listens on inside the container.
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

// tinyproxyEnsureInstalled installs tinyproxy if it is not already
// present. Mirrors the pattern used by tmuxEnsureInstalled in
// dotfiles.go: try without sudo first, then fall back to sudo,
// across multiple package managers.
var tinyproxyEnsureInstalled = `command -v tinyproxy >/dev/null 2>&1 || { echo '==> Installing tinyproxy...'; (apt-get update -qq && apt-get install -y -qq tinyproxy) 2>/dev/null || (sudo apt-get update -qq && sudo apt-get install -y -qq tinyproxy) 2>/dev/null || (apk add tinyproxy) 2>/dev/null || (sudo apk add tinyproxy) 2>/dev/null || (dnf install -y tinyproxy) 2>/dev/null || (sudo dnf install -y tinyproxy) 2>/dev/null || (yum install -y tinyproxy) 2>/dev/null || (sudo yum install -y tinyproxy) 2>/dev/null || { echo 'INSTALL_FAILED'; exit 1; }; }; `

// tinyproxyConfig is a minimal tinyproxy configuration written to
// /tmp/fleet-proxy.conf via printf. Uses CONNECT for HTTPS/WSS and
// allows connections from any source (traffic arrives via the port
// forward, not the open network).
//
//   - DisableViaHeader: prevents tinyproxy from rejecting requests that
//     loop back through itself (the Via-header self-loop check).
//   - XTinyproxy: disabled to avoid leaking internal headers.
var tinyproxyConfig = `Port 58888\nListen 0.0.0.0\nTimeout 600\nAllow 0.0.0.0/0\nMaxClients 100\nDisableViaHeader Yes\nXTinyproxy No\n`

// proxySetupScript is the single-line shell command that installs
// tinyproxy (if missing) and starts it idempotently.
//
//	Flow:
//	  1. Install tinyproxy via the first available package manager
//	     (with sudo fallback).
//	  2. If a tinyproxy process is already running, exit early.
//	  3. Write a minimal config to /tmp/fleet-proxy.conf via printf.
//	  4. Start tinyproxy as a daemon.
var proxySetupScript = tinyproxyEnsureInstalled +
	`if [ -f /tmp/fleet-proxy.pid ] && kill -0 $(cat /tmp/fleet-proxy.pid) 2>/dev/null; then echo 'ALREADY_RUNNING'; exit 0; fi; ` +
	`printf '` + tinyproxyConfig + `' > /tmp/fleet-proxy.conf; ` +
	`tinyproxy -c /tmp/fleet-proxy.conf && echo 'STARTED' || { echo 'START_FAILED'; exit 1; }`

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
//	  | tinyproxy in container|
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
			//    port to tinyproxy inside the container.
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

		// 3. Ensure tinyproxy is installed and running inside the container.
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
// script. It returns an error if tinyproxy cannot be installed or started.
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
		return fmt.Errorf("could not install tinyproxy (no supported package manager found)")
	case strings.Contains(status, "START_FAILED"):
		return fmt.Errorf("tinyproxy failed to start — check container logs")
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
