package fleet

import (
	"fmt"
	"time"
)

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

// ParseBackendType validates a CLI/backend string and returns its BackendType.
func ParseBackendType(value string) (BackendType, error) {
	switch BackendType(value) {
	case BackendDevcontainer:
		return BackendDevcontainer, nil
	case BackendCoder:
		return BackendCoder, nil
	case BackendCodespaces:
		return BackendCodespaces, nil
	default:
		return "", fmt.Errorf("invalid backend %q (valid: devcontainer, coder, codespaces)", value)
	}
}

// ValidateBackendType returns an error if bt is not a known backend.
func ValidateBackendType(bt BackendType) error {
	_, err := ParseBackendType(string(bt))
	return err
}

// Instance represents a single devcontainer workspace in a fleet.
//
// Name is the stable identifier used for container names, workspace paths,
// tmux session prefixes, and CLI lookups — it never changes after creation.
// DisplayName is the user-facing label shown in the TUI; it may be edited
// freely without touching any underlying resources.
type Instance struct {
	Name         string         `json:"name"`
	DisplayName  string         `json:"display_name,omitempty"`
	ContainerID  string         `json:"container_id"`
	Config       string         `json:"config"`
	WorkspaceDir string         `json:"workspace_dir"`
	CreatedAt    time.Time      `json:"created_at"`
	Status       InstanceStatus `json:"status"`
	Error        string         `json:"error,omitempty"`
	Backend      BackendType    `json:"backend,omitempty"`
	Tag          string         `json:"tag,omitempty"`
	Color        string         `json:"color,omitempty"`
	Branch       string         `json:"branch,omitempty"`
}

// GetDisplayName returns the user-facing label for the instance. Legacy
// instances persisted before DisplayName existed fall back to Name.
func (i *Instance) GetDisplayName() string {
	if i.DisplayName == "" {
		return i.Name
	}
	return i.DisplayName
}
