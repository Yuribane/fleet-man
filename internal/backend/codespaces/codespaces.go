package codespaces

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

// ===========================================
// Options
// ===========================================

// Option configures a CodespacesBackend.
type Option func(*CodespacesBackend)

// WithVerbose enables verbose output.
func WithVerbose(v bool) Option {
	return func(b *CodespacesBackend) { b.verbose = v }
}

// WithRepo sets the GitHub repository (owner/repo) for codespace creation.
func WithRepo(repo string) Option {
	return func(b *CodespacesBackend) { b.repo = repo }
}

// WithMachine sets the machine type for codespace creation.
func WithMachine(machine string) Option {
	return func(b *CodespacesBackend) { b.machine = machine }
}

// WithIdleTimeout sets the idle timeout duration string (e.g. "30m").
func WithIdleTimeout(timeout string) Option {
	return func(b *CodespacesBackend) { b.idleTimeout = timeout }
}

// WithDevcontainerPath sets the path to the devcontainer.json within the repo.
func WithDevcontainerPath(path string) Option {
	return func(b *CodespacesBackend) { b.devcontainerPath = path }
}

// ===========================================
// Backend
// ===========================================

// CodespacesBackend implements backend.Backend using the GitHub CLI (gh).
// The "containerID" for codespaces is the codespace name returned by
// `gh codespace create`.
type CodespacesBackend struct {
	verbose          bool
	repo             string // GitHub repo (owner/repo)
	machine          string // machine type (e.g. "basicLinux32gb")
	idleTimeout      string // idle timeout (e.g. "30m")
	devcontainerPath string // path to devcontainer.json in repo
}

// New creates a new CodespacesBackend.
func New(opts ...Option) *CodespacesBackend {
	b := &CodespacesBackend{}
	for _, o := range opts {
		o(b)
	}
	return b
}

// ===========================================
// Lifecycle
// ===========================================

// Up creates a GitHub Codespace. workspaceDir is used to derive the display
// name. The repo is set via WithRepo at construction time.
func (b *CodespacesBackend) Up(workspaceDir string) (*backend.UpResult, error) {
	displayName := codespaceName(workspaceDir)

	args := []string{"codespace", "create", "--repo", b.repo, "--display-name", displayName}
	if b.machine != "" {
		args = append(args, "--machine", b.machine)
	}
	if b.idleTimeout != "" {
		args = append(args, "--idle-timeout", b.idleTimeout)
	}
	if b.devcontainerPath != "" {
		args = append(args, "--devcontainer-path", b.devcontainerPath)
	}

	cmd := exec.Command("gh", args...)
	var stderrBuf strings.Builder
	if b.verbose {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	} else {
		cmd.Stderr = &stderrBuf
	}

	out, err := cmd.Output()
	if err != nil {
		stderr := stderrBuf.String()
		if isAuthScopeError(stderr) {
			return nil, fmt.Errorf("%s%s", ErrPrefixAuthScope, "gh auth token is missing the \"codespace\" scope")
		}
		if isCodespaceLimitError(stderr) {
			return nil, fmt.Errorf("%s%s", ErrPrefixLimit, "codespace limit reached")
		}
		return nil, fmt.Errorf("gh codespace create failed: %w", err)
	}

	csName := strings.TrimSpace(string(out))
	if csName == "" {
		return nil, fmt.Errorf("gh codespace create returned empty name")
	}

	// Wait for the codespace to become available.
	if err := b.waitForState(csName, "Available", 5*time.Minute); err != nil {
		return nil, err
	}

	return &backend.UpResult{
		Outcome:               "success",
		ContainerID:           csName,
		RemoteUser:            "codespace",
		RemoteWorkspaceFolder: "/workspaces",
	}, nil
}

// Down deletes a codespace permanently.
func (b *CodespacesBackend) Down(containerID string) error {
	cmd := exec.Command("gh", "codespace", "delete", "-c", containerID, "--force")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops a running codespace.
func (b *CodespacesBackend) Stop(containerID string) error {
	cmd := exec.Command("gh", "codespace", "stop", "-c", containerID)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Start resumes a stopped codespace using the GitHub REST API.
func (b *CodespacesBackend) Start(containerID string) error {
	cmd := exec.Command("gh", "api", "-X", "POST",
		fmt.Sprintf("/user/codespaces/%s/start", containerID))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh api start codespace: %w", err)
	}

	return b.waitForState(containerID, "Available", 5*time.Minute)
}

// ===========================================
// Execution
// ===========================================

// Exec runs an interactive command inside a codespace via SSH.
func (b *CodespacesBackend) Exec(workspaceDir string, command []string) error {
	name := codespaceName(workspaceDir)
	args := sshArgs(name, command)
	cmd := exec.Command("gh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ExecCommand returns an unstarted *exec.Cmd for running a command
// inside a codespace via SSH.
func (b *CodespacesBackend) ExecCommand(workspaceDir string, command []string) *exec.Cmd {
	name := codespaceName(workspaceDir)
	args := sshArgs(name, command)
	return exec.Command("gh", args...)
}

// ===========================================
// Monitoring
// ===========================================

// Stats returns CPU and memory usage for the given codespace names.
// Uses SSH to read process stats from each codespace concurrently.
func (b *CodespacesBackend) Stats(containerIDs []string) (map[string]*backend.ContainerStats, error) {
	if len(containerIDs) == 0 {
		return nil, nil
	}

	type statsResult struct {
		id    string
		stats *backend.ContainerStats
		err   error
	}

	ch := make(chan statsResult, len(containerIDs))
	for _, id := range containerIDs {
		go func(csName string) {
			s, err := b.fetchStats(csName)
			ch <- statsResult{csName, s, err}
		}(id)
	}

	result := make(map[string]*backend.ContainerStats)
	for range containerIDs {
		r := <-ch
		if r.err == nil && r.stats != nil {
			result[r.id] = r.stats
		}
	}

	return result, nil
}

// Logs streams codespace creation logs.
func (b *CodespacesBackend) Logs(containerID string, follow bool) error {
	args := []string{"codespace", "logs", "-c", containerID}
	if follow {
		args = append(args, "--follow")
	}

	cmd := exec.Command("gh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogsCommand returns an unstarted *exec.Cmd for streaming codespace logs.
func (b *CodespacesBackend) LogsCommand(containerID string, follow bool) *exec.Cmd {
	args := []string{"codespace", "logs", "-c", containerID}
	if follow {
		args = append(args, "--follow")
	}
	return exec.Command("gh", args...)
}

// CaptureScreen runs `tmux capture-pane` inside a codespace via SSH.
func (b *CodespacesBackend) CaptureScreen(containerID, tmuxSession string) backend.ScreenCapture {
	cmd := exec.Command("gh", sshArgs(containerID, []string{"tmux", "capture-pane", "-t", tmuxSession, "-p"})...)
	out, err := cmd.Output()
	if err != nil {
		return backend.ScreenCapture{}
	}
	return backend.ScreenCapture{Content: string(out), OK: true}
}

// AgentToolProbe detects which agent tool is running inside a codespace.
func (b *CodespacesBackend) AgentToolProbe(containerID string) (string, bool) {
	cmd := exec.Command("gh", sshArgs(containerID, []string{toolProbeScript})...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return parseToolProbeOutput(string(out))
}

// ===========================================
// Integration
// ===========================================

// EditorURI returns a VS Code URI for connecting to a GitHub Codespace.
func (b *CodespacesBackend) EditorURI(workspaceDir string, projectName string) (string, bool) {
	name := codespaceName(workspaceDir)
	uri := fmt.Sprintf("vscode://github.codespaces/connect?name=%s", name)
	return uri, true
}

// PortForwardCommand returns an unstarted *exec.Cmd that forwards localPort
// on the host to remotePort inside the codespace using `gh codespace ports forward`.
func (b *CodespacesBackend) PortForwardCommand(containerID string, localPort, remotePort int) *exec.Cmd {
	mapping := fmt.Sprintf("%d:%d", localPort, remotePort)
	return exec.Command("gh", "codespace", "ports", "forward", mapping, "-c", containerID)
}

// ResolveHostname returns ("", false) for GitHub Codespaces because they
// are remote and not directly reachable by IP from the host.
func (b *CodespacesBackend) ResolveHostname(containerID string) (string, bool) {
	return "", false
}

// ===========================================
// Internal helpers
// ===========================================

// codespaceInfo represents the JSON structure returned by `gh codespace view`.
type codespaceInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
}

// sshArgs builds the argument list for `gh codespace ssh`.
func sshArgs(csName string, command []string) []string {
	args := []string{"codespace", "ssh", "-c", csName}
	if len(command) == 0 {
		return args
	}
	args = append(args, "--")
	args = append(args, command...)
	return args
}

// getCodespace fetches codespace details via `gh codespace view`.
func (b *CodespacesBackend) getCodespace(name string) (*codespaceInfo, error) {
	cmd := exec.Command("gh", "codespace", "view", "-c", name, "--json", "name,displayName,state")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh codespace view: %w", err)
	}

	var info codespaceInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parsing codespace view output: %w", err)
	}

	return &info, nil
}

// waitForState polls until the codespace reaches the desired state or times out.
func (b *CodespacesBackend) waitForState(name, desiredState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		info, err := b.getCodespace(name)
		if err == nil && info.State == desiredState {
			return nil
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("codespace %q did not reach state %q within %v", name, desiredState, timeout)
}

// fetchStats reads CPU and memory stats from a codespace via SSH.
func (b *CodespacesBackend) fetchStats(csName string) (*backend.ContainerStats, error) {
	cmd := exec.Command("gh", sshArgs(csName, []string{"ps", "-eo", "pcpu=,rss=", "--no-headers"})...)
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
		CPUMillicores: totalCPU * 10,
		MemoryMB:      totalMem / 1024,
	}, nil
}
