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

// isCodespaceLimitError returns true if the stderr output from gh
// indicates the user has reached their maximum codespace count.
func isCodespaceLimitError(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "maximum number") ||
		strings.Contains(lower, "limit") && strings.Contains(lower, "codespace") ||
		strings.Contains(lower, "you have already reached") ||
		strings.Contains(lower, "out of codespaces")
}

// ===========================================
// Tool Probe
// ===========================================

// toolProbeScript is the shell script run inside each codespace to
// detect which agent tool is running. Formatted as a single line to
// avoid argument parsing issues with gh codespace ssh.
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
