package fleet

import "time"

type InstanceStatus string

const (
	StatusCreating InstanceStatus = "creating"
	StatusRunning  InstanceStatus = "running"
	StatusStopped  InstanceStatus = "stopped"
	StatusFailed   InstanceStatus = "failed"
)

type Instance struct {
	Name        string         `json:"name"`
	ContainerID string         `json:"container_id"`
	Config      string         `json:"config"`
	WorkspaceDir string        `json:"workspace_dir"`
	CreatedAt   time.Time      `json:"created_at"`
	Status      InstanceStatus `json:"status"`
	Error       string         `json:"error,omitempty"`
	Tag         string         `json:"tag,omitempty"`
}
