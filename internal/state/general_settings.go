package state

// GeneralSettings holds general user preferences.
type GeneralSettings struct {
	TmuxVimKeys  *bool `json:"tmux_vim_keys,omitempty"`  // nil = true (default on)
	ShowHelpText *bool `json:"show_help_text,omitempty"` // nil = true (default on)
}

// TmuxVimKeysEnabled reports whether vim-style tmux pane navigation is active.
func (g GeneralSettings) TmuxVimKeysEnabled() bool {
	if g.TmuxVimKeys == nil {
		return true
	}
	return *g.TmuxVimKeys
}

// ShowHelpTextEnabled reports whether the help bar is visible on the main page.
func (g GeneralSettings) ShowHelpTextEnabled() bool {
	if g.ShowHelpText == nil {
		return true
	}
	return *g.ShowHelpText
}
