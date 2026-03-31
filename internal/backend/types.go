package backend

// UpResult holds the outcome of provisioning a workspace.
type UpResult struct {
	Outcome               string `json:"outcome"`
	ContainerID           string `json:"containerId"`
	RemoteUser            string `json:"remoteUser"`
	RemoteWorkspaceFolder string `json:"remoteWorkspaceFolder"`
}

// ContainerStats holds CPU and memory usage for a container.
type ContainerStats struct {
	CPUMillicores float64
	MemoryMB      float64
}

// ScreenCapture holds the result of capturing a tmux pane's content.
type ScreenCapture struct {
	Content string // visible pane text; empty when capture failed
	OK      bool   // true if the tmux session exists and capture succeeded
}
