package backendutil

import (
	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	coderbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/coder"
	devcontainerbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
)

// New creates a Backend for the given type.
func New(bt fleet.BackendType, verbose bool) backend.Backend {
	switch bt {
	case fleet.BackendCoder:
		opts := []coderbackend.Option{}
		if verbose {
			opts = append(opts, coderbackend.WithVerbose(verbose))
		}
		return coderbackend.New(opts...)
	default:
		opts := []devcontainerbackend.Option{}
		if verbose {
			opts = append(opts, devcontainerbackend.WithVerbose(verbose))
		}
		return devcontainerbackend.New(opts...)
	}
}

// CoderOpenVSCodeArgs builds args for `coder open vscode`.
// containerID may be "workspace" or "workspace.agent" — both forms
// are accepted directly by the coder CLI.
func CoderOpenVSCodeArgs(containerID string) []string {
	return []string{"open", "vscode", containerID}
}
