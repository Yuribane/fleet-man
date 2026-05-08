package state

// CoderParameter holds a single Coder template parameter binding.
type CoderParameter struct {
	Name         string `json:"name"`
	Value        string `json:"value"`                   // may contain ${GIT_URL}, ${GIT_BRANCH}, ${INSTANCE_NAME}
	DefaultValue string `json:"default_value,omitempty"` // from template
	DisplayName  string `json:"display_name,omitempty"`  // from template
	Description  string `json:"description,omitempty"`   // from template
	Type         string `json:"type,omitempty"`          // "string", "number"
}
