package state

// CodespacesSettings holds GitHub Codespaces preferences.
type CodespacesSettings struct {
	Machine          string `json:"machine,omitempty"`           // machine type (e.g. "basicLinux32gb")
	IdleTimeout      string `json:"idle_timeout,omitempty"`      // duration (e.g. "30m")
	DevcontainerPath string `json:"devcontainer_path,omitempty"` // path to devcontainer.json in repo
}
