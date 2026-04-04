package deps

import "os/exec"

// Dependency describes a binary dependency and how to install it.
type Dependency struct {
	Name       string
	Binary     string
	Required   bool
	InstallURL string
	Found      bool
}

// Check looks up whether the devcontainer and coder binaries are
// available on the user's PATH and returns a report for each.
func Check() []Dependency {
	deps := []Dependency{
		{
			Name:       "devcontainer CLI",
			Binary:     "devcontainer",
			Required:   true,
			InstallURL: "https://github.com/devcontainers/cli#npm-install",
		},
		{
			Name:       "Coder CLI",
			Binary:     "coder",
			Required:   false,
			InstallURL: "https://coder.com/docs/install",
		},
	}

	for i := range deps {
		_, err := exec.LookPath(deps[i].Binary)
		deps[i].Found = err == nil
	}

	return deps
}

// HasMissing returns true if any dependency in the list was not found.
func HasMissing(deps []Dependency) bool {
	for _, d := range deps {
		if !d.Found {
			return true
		}
	}
	return false
}
