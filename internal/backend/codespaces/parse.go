package codespaces

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ===========================================
// Error Detection
// ===========================================

// ErrPrefixAuthScope is the prefix used in error messages when the gh
// auth token is missing the "codespace" scope.
const ErrPrefixAuthScope = "codespaces:auth_scope:"

// ErrPrefixLimit is the prefix used in error messages when the user
// has reached their codespace limit.
const ErrPrefixLimit = "codespaces:limit:"

// ErrPrefixMachine is the prefix used in error messages when gh needs
// an interactive machine type selection but has no terminal.
const ErrPrefixMachine = "codespaces:machine:"

// isAuthScopeError returns true if the stderr output from gh indicates
// an authentication problem — either not logged in or missing the
// "codespace" OAuth scope.
func isAuthScopeError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "gh auth login") ||
		strings.Contains(lower, "gh auth refresh") ||
		strings.Contains(lower, "codespace") && strings.Contains(lower, "scope") ||
		strings.Contains(lower, "http 403") && strings.Contains(lower, "scope")
}

// isMachineSelectionError returns true if the stderr output from gh
// indicates it tried to prompt for machine type but had no terminal.
func isMachineSelectionError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "no terminal") ||
		strings.Contains(lower, "machine type") && strings.Contains(lower, "error")
}

// isCodespaceLimitError returns true if the stderr output from gh
// indicates the user has reached their maximum codespace count.
func isCodespaceLimitError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "maximum number") ||
		strings.Contains(lower, "limit") && strings.Contains(lower, "codespace") ||
		strings.Contains(lower, "you have already reached") ||
		strings.Contains(lower, "out of codespaces")
}

// parseFloat parses a trimmed string into a float64.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// codespaceName derives a display name for a GitHub Codespace from a workspace dir path.
// workspaceDir format: ~/.fleet/workspaces/{fleet}/{instance}/{fleet}
// Returns "{fleet}-{instance}" sanitized for Codespace display names.
func codespaceName(workspaceDir string) string {
	parent := filepath.Dir(workspaceDir)       // .../workspaces/{fleet}/{instance}
	instance := filepath.Base(parent)           // {instance}
	grandparent := filepath.Dir(parent)         // .../workspaces/{fleet}
	fleetName := filepath.Base(grandparent)     // {fleet}

	name := fleetName + "-" + instance
	return sanitizeName(name)
}

// invalidNameChars matches characters not allowed in codespace display names.
var invalidNameChars = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeName produces a valid codespace display name: lowercase alphanumeric
// with hyphens, max 48 characters.
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = invalidNameChars.ReplaceAllString(name, "-")
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	if len(name) > 48 {
		name = name[:48]
	}
	name = strings.TrimRight(name, "-")
	if name == "" {
		name = "workspace"
	}
	return name
}
