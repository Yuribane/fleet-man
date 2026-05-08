package state

// DotfilesSettings holds dotfiles repository preferences.
type DotfilesSettings struct {
	RepoURL       string `json:"repo_url"`
	InstallScript string `json:"install_script"`
	AutoInstall   bool   `json:"auto_install"`
}
