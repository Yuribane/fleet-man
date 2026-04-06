package fleet

import "time"

type InstanceStatus string

const (
	StatusCreating InstanceStatus = "creating"
	StatusRunning  InstanceStatus = "running"
	StatusStopped  InstanceStatus = "stopped"
	StatusFailed   InstanceStatus = "failed"
	StatusStopping InstanceStatus = "stopping"
	StatusStarting InstanceStatus = "starting"
	StatusDeleting InstanceStatus = "deleting"
)

// BackendType identifies which backend an instance uses.
type BackendType string

const (
	BackendDevcontainer BackendType = "devcontainer"
	BackendCoder        BackendType = "coder"
	BackendCodespaces   BackendType = "codespaces"
)

type Instance struct {
	Name        string         `json:"name"`
	ContainerID string         `json:"container_id"`
	Config      string         `json:"config"`
	WorkspaceDir string        `json:"workspace_dir"`
	CreatedAt   time.Time      `json:"created_at"`
	Status      InstanceStatus `json:"status"`
	Error       string         `json:"error,omitempty"`
	Backend     BackendType    `json:"backend,omitempty"`
	Tag         string         `json:"tag,omitempty"`
}
