package coder

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
)

// Option configures a CoderBackend.
type Option func(*CoderBackend)

// WithVerbose enables verbose output.
func WithVerbose(v bool) Option {
	return func(b *CoderBackend) { b.verbose = v }
}

// WithTemplate sets the Coder template to use when creating workspaces.
func WithTemplate(t string) Option {
	return func(b *CoderBackend) { b.template = t }
}

// WithPreset sets the Coder preset to use when creating workspaces.
func WithPreset(p string) Option {
	return func(b *CoderBackend) { b.preset = p }
}

// WithParameters sets the resolved parameter key-value pairs for workspace creation.
func WithParameters(params map[string]string) Option {
	return func(b *CoderBackend) { b.parameters = params }
}

// CoderBackend implements backend.Backend using the Coder CLI.
// The "containerID" for coder workspaces is the workspace name.
type CoderBackend struct {
	verbose    bool
	template   string            // coder template name
	preset     string            // coder preset name
	parameters map[string]string // resolved parameter key=value pairs
}

// New creates a new CoderBackend.
func New(opts ...Option) *CoderBackend {
	b := &CoderBackend{}
	for _, o := range opts {
		o(b)
	}
	return b
}

// coderAgent represents a single agent in a coder workspace.
type coderAgent struct {
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	LifecycleState    string  `json:"lifecycle_state"`
	ParentID          *string `json:"parent_id"` // non-nil for devcontainer agents
	Directory         string  `json:"directory"`
	ExpandedDirectory string  `json:"expanded_directory"`
}

// coderWorkspace is the JSON structure returned by `coder list -o json`.
type coderWorkspace struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	TemplateName string `json:"template_name"`
	LatestBuild  struct {
		Status    string `json:"status"`
		Resources []struct {
			Agents []coderAgent `json:"agents"`
		} `json:"resources"`
	} `json:"latest_build"`
}

// Up creates a Coder workspace. workspaceDir is used to derive the workspace
// name (last path component). The git clone happens inside the Coder template
// via the repo parameter.
func (b *CoderBackend) Up(workspaceDir string) (*backend.UpResult, error) {
	// Derive workspace name from the workspace dir path.
	// workspaceDir format: ~/.fleet/workspaces/{fleet}/{instance}/{fleet}
	// We need a unique, valid coder workspace name.
	name := coderWorkspaceName(workspaceDir)

	args := []string{"create", name, "--yes"}
	if b.template != "" {
		args = append(args, "--template", b.template)
	}
	if b.preset != "" {
		args = append(args, "--preset", b.preset)
	}
	for k, v := range b.parameters {
		args = append(args, "--parameter", k+"="+v)
	}

	cmd := exec.Command("coder", args...)
	if b.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("coder create failed: %w", err)
	}

	// Wait for the agent to be connected and its startup script to finish.
	remoteDir, err := b.waitForAgent(name)
	if err != nil {
		return nil, err
	}

	// Detect and provision nested devcontainer if present.
	b.maybeDevcontainerUp(name, remoteDir)

	// Check if a devcontainer agent is now available. If so, use
	// "workspace.agent" as the container ID so all SSH calls route
	// to the devcontainer instead of the outer workspace.
	sshTarget := name
	if dcAgent := b.findDevcontainerAgent(name); dcAgent != "" {
		sshTarget = name + "." + dcAgent
	}

	return &backend.UpResult{
		Outcome:               "success",
		ContainerID:           sshTarget,
		RemoteUser:            "coder",
		RemoteWorkspaceFolder: remoteDir,
	}, nil
}

// workspaceName extracts the workspace name from a containerID which may
// be in "workspace.agent" format. Coder lifecycle commands (stop, start,
// delete) operate on the workspace, not individual agents.
func workspaceName(containerID string) string {
	if i := strings.Index(containerID, "."); i >= 0 {
		return containerID[:i]
	}
	return containerID
}

// resolveSSHTarget returns the best SSH target for a workspace. If the
// workspace has a connected devcontainer agent, returns "workspace.agent".
// Otherwise returns the containerID as-is. This ensures SSH always routes
// to the devcontainer when one exists.
func (b *CoderBackend) resolveSSHTarget(containerID string) string {
	// Already has an agent suffix — use as-is
	if strings.Contains(containerID, ".") {
		return containerID
	}
	if agent := b.findDevcontainerAgent(containerID); agent != "" {
		return containerID + "." + agent
	}
	return containerID
}

// Down deletes a Coder workspace permanently.
func (b *CoderBackend) Down(containerID string) error {
	cmd := exec.Command("coder", "delete", "--yes", workspaceName(containerID))
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops a Coder workspace.
func (b *CoderBackend) Stop(containerID string) error {
	cmd := exec.Command("coder", "stop", "--yes", workspaceName(containerID))
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Start starts a stopped Coder workspace.
func (b *CoderBackend) Start(containerID string) error {
	cmd := exec.Command("coder", "start", "--yes", workspaceName(containerID))
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Exec runs an interactive command inside a Coder workspace via SSH.
func (b *CoderBackend) Exec(workspaceDir string, command []string) error {
	name := coderWorkspaceName(workspaceDir)
	target := b.resolveSSHTarget(name)
	args := sshArgs(target, command)
	cmd := exec.Command("coder", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ExecCommand returns an unstarted *exec.Cmd for running a command
// inside a Coder workspace via SSH.
func (b *CoderBackend) ExecCommand(workspaceDir string, command []string) *exec.Cmd {
	name := coderWorkspaceName(workspaceDir)
	target := b.resolveSSHTarget(name)
	args := sshArgs(target, command)
	return exec.Command("coder", args...)
}

// Stats returns CPU and memory usage for the given workspace IDs (names).
// Uses SSH to read /proc stats from each workspace concurrently.
func (b *CoderBackend) Stats(containerIDs []string) (map[string]*backend.ContainerStats, error) {
	if len(containerIDs) == 0 {
		return nil, nil
	}

	result := make(map[string]*backend.ContainerStats)
	type statsResult struct {
		id    string
		stats *backend.ContainerStats
		err   error
	}

	ch := make(chan statsResult, len(containerIDs))
	for _, id := range containerIDs {
		go func(wsName string) {
			s, err := b.fetchWorkspaceStats(wsName)
			ch <- statsResult{wsName, s, err}
		}(id)
	}

	for range containerIDs {
		r := <-ch
		if r.err == nil && r.stats != nil {
			result[r.id] = r.stats
		}
	}

	return result, nil
}

// Logs streams workspace build logs.
func (b *CoderBackend) Logs(containerID string, follow bool) error {
	args := []string{"logs", workspaceName(containerID)}
	if follow {
		args = append(args, "--follow")
	}

	cmd := exec.Command("coder", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogsCommand returns an unstarted *exec.Cmd for streaming workspace logs.
func (b *CoderBackend) LogsCommand(containerID string, follow bool) *exec.Cmd {
	args := []string{"logs", workspaceName(containerID)}
	if follow {
		args = append(args, "--follow")
	}
	return exec.Command("coder", args...)
}

// CaptureScreen runs `tmux capture-pane` inside a Coder workspace via SSH.
func (b *CoderBackend) CaptureScreen(containerID, tmuxSession string) backend.ScreenCapture {
	target := b.resolveSSHTarget(containerID)
	cmd := exec.Command("coder", sshArgs(target, []string{"tmux", "capture-pane", "-t", tmuxSession, "-p"})...)
	out, err := cmd.Output()
	if err != nil {
		return backend.ScreenCapture{}
	}
	return backend.ScreenCapture{Content: string(out), OK: true}
}

// AgentToolProbe detects which agent tool is running inside a Coder workspace.
func (b *CoderBackend) AgentToolProbe(containerID string) (string, bool) {
	// coder ssh wraps everything after -- in a shell invocation, so we
	// pass the script directly rather than via sh -c.
	target := b.resolveSSHTarget(containerID)
	cmd := exec.Command("coder", sshArgs(target, []string{toolProbeScript})...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return parseToolProbeOutput(string(out))
}

// EditorURI returns a VS Code URI for connecting to a Coder workspace.
func (b *CoderBackend) EditorURI(workspaceDir string, projectName string) (string, bool) {
	name := coderWorkspaceName(workspaceDir)
	// VS Code Coder extension uses vscode://coder.coder-remote/open?... format
	// but the simpler approach is to use `coder open vscode` which handles it.
	// Return the workspace name so the CLI can use `coder open vscode <name>`.
	uri := "coder-vscode://" + name
	return uri, true
}

// getWorkspace fetches workspace details via `coder list -o json`.
func (b *CoderBackend) getWorkspace(name string) (*coderWorkspace, error) {
	cmd := exec.Command("coder", "list", "-o", "json", "--search", "name:"+name)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("coder list: %w", err)
	}

	var workspaces []coderWorkspace
	if err := json.Unmarshal(out, &workspaces); err != nil {
		return nil, fmt.Errorf("parsing coder list output: %w", err)
	}

	for i := range workspaces {
		if workspaces[i].Name == name {
			return &workspaces[i], nil
		}
	}

	return nil, fmt.Errorf("workspace %q not found", name)
}

// waitForAgent polls until the coder agent is connected and its startup
// script has finished (lifecycle_state == "ready"). Returns the agent's
// working directory. Times out after 5 minutes.
func (b *CoderBackend) waitForAgent(wsName string) (string, error) {
	deadline := time.Now().Add(5 * time.Minute)
	remoteDir := "/workspaces"

	for time.Now().Before(deadline) {
		ws, err := b.getWorkspace(wsName)
		if err == nil {
			for _, r := range ws.LatestBuild.Resources {
				for _, a := range r.Agents {
					if a.ExpandedDirectory != "" {
						remoteDir = a.ExpandedDirectory
					}
					if a.Status == "connected" && a.LifecycleState == "ready" {
						return remoteDir, nil
					}
				}
			}
		}
		time.Sleep(3 * time.Second)
	}

	// Timed out but return what we have — the workspace may still be usable
	return remoteDir, nil
}

// maybeDevcontainerUp checks if a .devcontainer directory exists under the
// workspace folder and runs `devcontainer up` inside the coder workspace if
// found. This handles the common "nested devcontainer" pattern where a coder
// template provisions a VM/pod and the repo inside contains a devcontainer.
func (b *CoderBackend) maybeDevcontainerUp(wsName, remoteDir string) {
	// Find repos with .devcontainer under the workspace directory.
	// Use 2>/dev/null to suppress permission errors (e.g. lost+found)
	// that would cause a non-zero exit despite valid output.
	findCmd := exec.Command("coder", sshArgs(wsName, []string{
		"find", remoteDir, "-maxdepth", "2", "-name", ".devcontainer", "-type", "d", "-print", "-quit",
	})...)
	out, _ := findCmd.Output() // ignore exit code — permission errors are expected

	dcPath := strings.TrimSpace(string(out))
	if dcPath == "" {
		return
	}

	// Take the first match and derive the workspace folder (parent of .devcontainer)
	lines := strings.Split(dcPath, "\n")
	wsFolder := strings.TrimSuffix(strings.TrimSpace(lines[0]), "/.devcontainer")
	if wsFolder == "" {
		return
	}

	// Run devcontainer up inside the coder workspace
	dcUp := exec.Command("coder", sshArgs(wsName, []string{
		"devcontainer", "up", "--workspace-folder", wsFolder,
	})...)
	if b.verbose {
		dcUp.Stdout = os.Stdout
		dcUp.Stderr = os.Stderr
	}
	_ = dcUp.Run()
}

// findDevcontainerAgent checks if the workspace has a connected devcontainer
// agent (identified by having a non-nil parent_id). Returns the agent name
// or empty string if none found.
func (b *CoderBackend) findDevcontainerAgent(wsName string) string {
	ws, err := b.getWorkspace(wsName)
	if err != nil {
		return ""
	}
	for _, r := range ws.LatestBuild.Resources {
		for _, a := range r.Agents {
			if a.ParentID != nil && a.Status == "connected" {
				return a.Name
			}
		}
	}
	return ""
}

// fetchWorkspaceStats reads CPU and memory stats from a workspace via SSH.
func (b *CoderBackend) fetchWorkspaceStats(wsName string) (*backend.ContainerStats, error) {
	// Simple approach: use ps output piped through awk, but run as
	// a single shell command to avoid escaping issues.
	target := b.resolveSSHTarget(wsName)
	cmd := exec.Command("coder", sshArgs(target, []string{"ps", "-eo", "pcpu=,rss=", "--no-headers"})...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var totalCPU, totalMem float64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		cpu, _ := parseFloat(fields[0])
		mem, _ := parseFloat(fields[1])
		totalCPU += cpu
		totalMem += mem
	}

	return &backend.ContainerStats{
		CPUMillicores: totalCPU * 10, // convert percentage to millicores
		MemoryMB:      totalMem / 1024,
	}, nil
}

// sshArgs builds the argument list for `coder ssh`.
// coder ssh concatenates everything after -- and passes it to the remote
// shell, so "sh -c 'script'" must be collapsed to just "script" to
// avoid double-shell wrapping.
func sshArgs(wsName string, command []string) []string {
	args := []string{"ssh"}
	// Forward SSH agent when available on the host.
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		args = append(args, "-A")
	}
	args = append(args, wsName)
	if len(command) == 0 {
		return args
	}
	args = append(args, "--")
	// Collapse "sh -c 'script'" into just "script" since coder ssh
	// already wraps in a shell.
	if len(command) == 3 && command[0] == "sh" && command[1] == "-c" {
		args = append(args, command[2])
	} else {
		args = append(args, command...)
	}
	return args
}
