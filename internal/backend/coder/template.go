package coder

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RichParameter represents a template parameter from the Coder API.
type RichParameter struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description"`
	Type         string   `json:"type"` // "string", "number"
	DefaultValue string   `json:"default_value"`
	Options      []ParamOption `json:"options"`
	Required     bool     `json:"required"`
	Mutable      bool     `json:"mutable"`
}

// ParamOption represents an allowed value for a parameter.
type ParamOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Preset represents a template version preset from the Coder API.
type Preset struct {
	ID          string         `json:"ID"`
	Name        string         `json:"Name"`
	Parameters  []PresetParam  `json:"Parameters"`
	Description string         `json:"Description"`
}

// PresetParam is a key-value parameter inside a preset.
type PresetParam struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// templateListEntry mirrors the coder templates list JSON output.
type templateListEntry struct {
	Template struct {
		Name            string `json:"name"`
		ActiveVersionID string `json:"active_version_id"`
	} `json:"Template"`
}

// FetchActiveVersionID returns the active template version ID for a given template name.
func FetchActiveVersionID(templateName string) (string, error) {
	cmd := exec.Command("coder", "templates", "list", "-o", "json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("coder templates list: %w", err)
	}

	var entries []templateListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return "", fmt.Errorf("parsing templates list: %w", err)
	}

	for _, e := range entries {
		if e.Template.Name == templateName {
			return e.Template.ActiveVersionID, nil
		}
	}

	return "", fmt.Errorf("template %q not found", templateName)
}

// FetchRichParameters fetches the rich parameters for a template version via REST API.
func FetchRichParameters(versionID string) ([]RichParameter, error) {
	body, err := coderAPIGet(fmt.Sprintf("api/v2/templateversions/%s/rich-parameters", versionID))
	if err != nil {
		return nil, err
	}

	var params []RichParameter
	if err := json.Unmarshal(body, &params); err != nil {
		return nil, fmt.Errorf("parsing rich parameters: %w", err)
	}
	return params, nil
}

// FetchPresets fetches the presets for a template version via REST API.
func FetchPresets(versionID string) ([]Preset, error) {
	body, err := coderAPIGet(fmt.Sprintf("api/v2/templateversions/%s/presets", versionID))
	if err != nil {
		return nil, err
	}

	var presets []Preset
	if err := json.Unmarshal(body, &presets); err != nil {
		return nil, fmt.Errorf("parsing presets: %w", err)
	}
	return presets, nil
}

// coderAPIGet makes an authenticated GET request to the Coder API.
func coderAPIGet(path string) ([]byte, error) {
	baseURL, token, err := coderCredentials()
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(baseURL, "/") + "/" + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Coder-Session-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coder API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coder API returned %d: %s", resp.StatusCode, body)
	}

	return body, nil
}

// coderCredentials reads the Coder URL and session token from the CLI config.
func coderCredentials() (string, string, error) {
	baseURL := os.Getenv("CODER_URL")
	token := os.Getenv("CODER_SESSION_TOKEN")

	if baseURL == "" || token == "" {
		configDir := coderConfigDir()
		if baseURL == "" {
			data, err := os.ReadFile(filepath.Join(configDir, "url"))
			if err != nil {
				return "", "", fmt.Errorf("coder not authenticated: cannot read URL from %s: %w", configDir, err)
			}
			baseURL = strings.TrimSpace(string(data))
		}
		if token == "" {
			data, err := os.ReadFile(filepath.Join(configDir, "session"))
			if err != nil {
				return "", "", fmt.Errorf("coder not authenticated: cannot read session from %s: %w", configDir, err)
			}
			token = strings.TrimSpace(string(data))
		}
	}

	return baseURL, token, nil
}

// coderConfigDir returns the coder CLI config directory.
func coderConfigDir() string {
	if d := os.Getenv("CODER_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "coderv2")
}
