package coder

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

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
