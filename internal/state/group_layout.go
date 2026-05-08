package state

// GroupLayout records the outer tmux pane layout for a session group so
// it can be restored after a fleet restart. The Layout field is a tmux
// window_layout string (geometry + pane sizes); Sessions preserves the
// pane-to-session mapping by screen position.
type GroupLayout struct {
	GroupID      string   `json:"groupID"`
	InstanceName string   `json:"instanceName"`
	Sessions     []string `json:"sessions"`
	Layout       string   `json:"layout"`
	PaneCount    int      `json:"paneCount"`
}
