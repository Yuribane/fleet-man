package devcontainer

// UpResult is the parsed JSON output from `devcontainer up`.
type UpResult struct {
	Outcome               string `json:"outcome"`
	ContainerID            string `json:"containerId"`
	RemoteUser             string `json:"remoteUser"`
	RemoteWorkspaceFolder  string `json:"remoteWorkspaceFolder"`
}

// ContainerStats holds CPU and memory usage for a container.
type ContainerStats struct {
	CPUMillicores float64
	MemoryMB      float64
}
