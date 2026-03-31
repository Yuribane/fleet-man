package coder

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

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

// coderWorkspace is the JSON structure returned by `coder list -o json`.
type coderWorkspace struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	TemplateName string `json:"template_name"`
	LatestBuild  struct {
		Status    string `json:"status"`
		Resources []struct {
			Agents []struct {
				Name              string `json:"name"`
				Status            string `json:"status"`
				Directory         string `json:"directory"`
				ExpandedDirectory string `json:"expanded_directory"`
			} `json:"agents"`
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

	// Fetch workspace details to get agent info
	ws, err := b.getWorkspace(name)
	if err != nil {
		return nil, fmt.Errorf("coder workspace created but failed to fetch details: %w", err)
	}

	remoteDir := "/workspaces"
	remoteUser := "coder"
	if len(ws.LatestBuild.Resources) > 0 {
		for _, r := range ws.LatestBuild.Resources {
			if len(r.Agents) > 0 {
				if d := r.Agents[0].ExpandedDirectory; d != "" {
					remoteDir = d
				}
				break
			}
		}
	}

	return &backend.UpResult{
		Outcome:               "success",
		ContainerID:           name, // workspace name is the ID for coder
		RemoteUser:            remoteUser,
		RemoteWorkspaceFolder: remoteDir,
	}, nil
}

// Down deletes a Coder workspace permanently.
func (b *CoderBackend) Down(containerID string) error {
	cmd := exec.Command("coder", "delete", "--yes", containerID)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops a Coder workspace.
func (b *CoderBackend) Stop(containerID string) error {
	cmd := exec.Command("coder", "stop", "--yes", containerID)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Start starts a stopped Coder workspace.
func (b *CoderBackend) Start(containerID string) error {
	cmd := exec.Command("coder", "start", "--yes", containerID)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Exec runs an interactive command inside a Coder workspace via SSH.
func (b *CoderBackend) Exec(workspaceDir string, command []string) error {
	name := coderWorkspaceName(workspaceDir)
	args := sshArgs(name, command)
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
	args := sshArgs(name, command)
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
	args := []string{"logs", containerID}
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
	args := []string{"logs", containerID}
	if follow {
		args = append(args, "--follow")
	}
	return exec.Command("coder", args...)
}

// CaptureScreen runs `tmux capture-pane` inside a Coder workspace via SSH.
func (b *CoderBackend) CaptureScreen(containerID, tmuxSession string) backend.ScreenCapture {
	cmd := exec.Command("coder", "ssh", containerID, "--", "tmux", "capture-pane", "-t", tmuxSession, "-p")
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
	cmd := exec.Command("coder", "ssh", containerID, "--", toolProbeScript)
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

// fetchWorkspaceStats reads CPU and memory stats from a workspace via SSH.
func (b *CoderBackend) fetchWorkspaceStats(wsName string) (*backend.ContainerStats, error) {
	// Simple approach: use ps output piped through awk, but run as
	// a single shell command to avoid escaping issues.
	cmd := exec.Command("coder", "ssh", wsName, "--", "ps", "-eo", "pcpu=,rss=", "--no-headers")
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
