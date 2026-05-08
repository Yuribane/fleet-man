package state

// CoderSettings holds Coder deployment preferences.
type CoderSettings struct {
	Template   string           `json:"template"`
	Preset     string           `json:"preset,omitempty"`
	Parameters []CoderParameter `json:"parameters,omitempty"`
}
