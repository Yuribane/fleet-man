package state

// AgentSettings holds AI agent preferences.
type AgentSettings struct {
	ToolSelection AgentTool `json:"tool_selection"`
}
