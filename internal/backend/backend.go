package backend

import "os/exec"

// Backend defines the strategy interface for container runtimes.
// Implementations handle provisioning, lifecycle, execution,
// monitoring, and introspection of containerized workspaces.
type Backend interface {
	// Up creates and starts a workspace from a workspace directory.
	Up(workspaceDir string) (*UpResult, error)

	// Down stops and removes a container permanently.
	Down(containerID string) error

	// Stop halts a running container without removing it.
	Stop(containerID string) error

	// Start resumes a previously stopped container.
	Start(containerID string) error

	// Exec runs an interactive command inside a running workspace.
	// Stdin, stdout, and stderr are connected to the caller's terminal.
	Exec(workspaceDir string, command []string) error

	// ExecCommand returns an unstarted *exec.Cmd for running a command
	// inside a workspace. The caller controls stdio and lifecycle.
	ExecCommand(workspaceDir string, command []string) *exec.Cmd

	// Stats returns CPU and memory usage for the given container IDs.
	Stats(containerIDs []string) (map[string]*ContainerStats, error)

	// Logs streams container logs to os.Stdout/os.Stderr synchronously.
	Logs(containerID string, follow bool) error

	// LogsCommand returns an unstarted *exec.Cmd for streaming logs.
	LogsCommand(containerID string, follow bool) *exec.Cmd

	// CaptureScreen captures the visible content of a tmux session
	// running inside a container.
	CaptureScreen(containerID, tmuxSession string) ScreenCapture

	// AgentToolProbe detects which agent tool (if any) is running
	// inside a container. Returns (tool, true) on success,
	// ("", false) on probe failure.
	AgentToolProbe(containerID string) (string, bool)

	// EditorURI returns a URI string that an editor (e.g. VS Code)
	// can use to connect to this workspace. Returns ("", false) if
	// editor integration is not supported by this backend.
	EditorURI(workspaceDir string, projectName string) (string, bool)

	// PortForwardCommand returns an unstarted *exec.Cmd that tunnels
	// traffic from localPort on the host to remotePort inside the
	// container/workspace. The process runs until killed by the caller.
	PortForwardCommand(containerID string, localPort, remotePort int) *exec.Cmd
}
