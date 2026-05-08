package deps

import "os/exec"

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
