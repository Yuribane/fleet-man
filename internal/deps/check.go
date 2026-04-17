package deps

import "os/exec"

// Dependency describes a binary dependency and how to install it.
type Dependency struct {
	Name        string
	Binary      string
	Required    bool
	InstallURL  string
	Description string
	Found       bool
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
		{
			Name:        "wl-clipboard",
			Binary:      "wl-copy",
			Required:    false,
			InstallURL:  wlClipboardInstallURL(),
			Description: "Copy/paste support on Wayland (only one of wl-clipboard or xclip needed).",
		},
		{
			Name:        "xclip",
			Binary:      "xclip",
			Required:    false,
			InstallURL:  "https://github.com/astrand/xclip",
			Description: "Copy/paste support on X11 (only one of wl-copy or xclip needed).",
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

// ToolStatus describes an optional tool, its install state, and purpose.
type ToolStatus struct {
	Name        string
	Binary      string
	Description string
	InstallURL  string
	Found       bool
}

// CheckTools returns the install status of optional tools that unlock
// additional fleet-man capabilities.
func CheckTools() []ToolStatus {
	tools := []ToolStatus{
		{
			Name:        "devcontainer",
			Binary:      "devcontainer",
			Description: "Adds local devcontainer development.",
			InstallURL:  "https://github.com/devcontainers/cli#npm-install",
		},
		{
			Name:        "gh",
			Binary:      "gh",
			Description: "Adds remote codespace development.",
			InstallURL:  "https://cli.github.com",
		},
		{
			Name:        "coder",
			Binary:      "coder",
			Description: "Adds remote coder workspace development.",
			InstallURL:  "https://coder.com/docs/install",
		},
		{
			Name:        "wl-clipboard",
			Binary:      "wl-copy",
			Description: "Optional — copy/paste support on Wayland (only one of wl-clipboard or xclip needed).",
			InstallURL:  wlClipboardInstallURL(),
		},
		{
			Name:        "xclip",
			Binary:      "xclip",
			Description: "Optional — copy/paste support on X11 (only one of wl-copy or xclip needed).",
			InstallURL:  "https://github.com/astrand/xclip",
		},
	}

	for i := range tools {
		_, err := exec.LookPath(tools[i].Binary)
		tools[i].Found = err == nil
	}

	return tools
}
