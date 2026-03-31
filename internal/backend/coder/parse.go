package coder

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// toolProbeScript is the shell script run inside each workspace to
// detect which agent tool is running. Same as the devcontainer backend
// but formatted as a single line to avoid coder ssh argument parsing issues.
const toolProbeScript = `for t in claude copilot codex gemini; do pids=$(ps aux 2>/dev/null | awk -v t="$t" '($11 ~ "(^|/)"t"$" || $12 ~ "(^|/)"t"$") {print $2}'); [ -n "$pids" ] && { echo "$t"; exit 0; }; done; echo "-"`

// parseToolProbeOutput parses the tool probe script output.
func parseToolProbeOutput(output string) (string, bool) {
	tool := strings.TrimSpace(output)
	if tool == "" {
		return "", false
	}
	if tool == "-" {
		return "", true
	}
	return tool, true
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// coderWorkspaceName derives a valid Coder workspace name from a workspace dir path.
// workspaceDir format: ~/.fleet/workspaces/{fleet}/{instance}/{fleet}
// Returns "{fleet}-{instance}" as the workspace name, sanitized for Coder.
func coderWorkspaceName(workspaceDir string) string {
	// Walk up the path to extract fleet and instance names
	// workspaceDir = .../workspaces/{fleet}/{instance}/{fleet}
	parent := filepath.Dir(workspaceDir)      // .../workspaces/{fleet}/{instance}
	instance := filepath.Base(parent)          // {instance}
	grandparent := filepath.Dir(parent)        // .../workspaces/{fleet}
	fleet := filepath.Base(grandparent)        // {fleet}

	name := fleet + "-" + instance
	return sanitizeCoderName(name)
}

// validCoderName matches Coder's workspace name constraints:
// lowercase alphanumeric + hyphens, must start/end with alphanumeric, max 32 chars.
var invalidCoderChars = regexp.MustCompile(`[^a-z0-9-]`)

func sanitizeCoderName(name string) string {
	name = strings.ToLower(name)
	name = invalidCoderChars.ReplaceAllString(name, "-")
	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	if len(name) > 32 {
		name = name[:32]
	}
	name = strings.TrimRight(name, "-")
	if name == "" {
		name = "workspace"
	}
	return name
}
