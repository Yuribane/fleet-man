package fleet

import "fmt"

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

// ValidateBackendType returns an error if backendType is not a known backend.
func ValidateBackendType(backendType BackendType) error {
	_, err := ParseBackendType(string(backendType))
	return err
}
