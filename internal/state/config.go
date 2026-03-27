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

// AllAgentToolNames returns the process names for all supported agent tools.
func AllAgentToolNames() []string {
	return []string{
		string(AgentToolClaude),
		string(AgentToolCodex),
		string(AgentToolCopilot),
		string(AgentToolGemini),
	}
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

// Config holds user preferences.
type Config struct {
	AgentSettings    AgentSettings    `json:"agent_settings"`
	DotfilesSettings DotfilesSettings `json:"dotfiles_settings"`
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
