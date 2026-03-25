package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds user preferences.
type Config struct{}

// ConfigPath returns the path to the config file.
func ConfigPath() string {
	return filepath.Join(FleetDir(), "config.json")
}

// LoadConfig reads the config from disk. Returns defaults if the file doesn't exist.
func LoadConfig() (*Config, error) {
	c := &Config{}

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

	return c, nil
}

// SaveConfig writes the config to disk.
func SaveConfig(c *Config) error {
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
