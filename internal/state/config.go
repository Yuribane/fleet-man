package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AgentTool string

const (
	AgentToolCodex   AgentTool = "codex"
	AgentToolClaude  AgentTool = "claude"
	AgentToolGemini  AgentTool = "gemini"
	AgentToolCopilot AgentTool = "copilot"
)

var validAgentTools = map[AgentTool]struct{}{
	AgentToolCodex:   {},
	AgentToolClaude:  {},
	AgentToolGemini:  {},
	AgentToolCopilot: {},
}

// AgentSettings holds AI agent preferences.
type AgentSettings struct {
	ToolSelection AgentTool `json:"tool_selection"`
}

// DotfilesSettings holds dotfiles repository preferences.
type DotfilesSettings struct {
	RepoURL       string `json:"repo_url"`
	InstallScript string `json:"install_script"`
}

// CoderParameter holds a single Coder template parameter binding.
type CoderParameter struct {
	Name         string `json:"name"`
	Value        string `json:"value"`                    // may contain ${GIT_URL}, ${INSTANCE_NAME}
	DefaultValue string `json:"default_value,omitempty"`  // from template
	DisplayName  string `json:"display_name,omitempty"`   // from template
	Description  string `json:"description,omitempty"`    // from template
	Type         string `json:"type,omitempty"`           // "string", "number"
}

// CoderSettings holds Coder deployment preferences.
type CoderSettings struct {
	Template   string           `json:"template"`
	Preset     string           `json:"preset,omitempty"`
	Parameters []CoderParameter `json:"parameters,omitempty"`
}

// Config holds user preferences.
type Config struct {
	AgentSettings    AgentSettings    `json:"agent_settings"`
	DotfilesSettings DotfilesSettings `json:"dotfiles_settings"`
	CoderSettings    CoderSettings    `json:"coder_settings"`
	DefaultBackend   string           `json:"default_backend,omitempty"` // "devcontainer" or "coder"
}

// DefaultConfig returns a config with default values applied.
func DefaultConfig() *Config {
	return &Config{
		AgentSettings: AgentSettings{
			ToolSelection: AgentToolClaude,
		},
	}
}

func (c *Config) applyDefaults() {
	if c == nil {
		return
	}

	if _, ok := validAgentTools[c.AgentSettings.ToolSelection]; !ok {
		c.AgentSettings.ToolSelection = AgentToolClaude
	}

	c.DotfilesSettings.RepoURL = strings.TrimSpace(c.DotfilesSettings.RepoURL)
	c.DotfilesSettings.InstallScript = strings.TrimSpace(c.DotfilesSettings.InstallScript)

	c.CoderSettings.Template = strings.TrimSpace(c.CoderSettings.Template)
	c.CoderSettings.Preset = strings.TrimSpace(c.CoderSettings.Preset)
}

// ConfigPath returns the path to the config file.
func ConfigPath() string {
	return filepath.Join(FleetDir(), "config.json")
}

// LoadConfig reads the config from disk. Returns defaults if the file doesn't exist.
func LoadConfig() (*Config, error) {
	c := DefaultConfig()

	data, err := os.ReadFile(ConfigPath())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	c.applyDefaults()
	return c, nil
}

// SaveConfig writes the config to disk.
func SaveConfig(c *Config) error {
	if c == nil {
		c = DefaultConfig()
	}
	c.applyDefaults()

	if err := os.MkdirAll(FleetDir(), 0755); err != nil {
		return fmt.Errorf("creating fleet dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(ConfigPath(), data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}
