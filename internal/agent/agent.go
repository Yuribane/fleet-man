// Package agent provides detection and invocation of coding-agent CLI tools
// (Claude Code, Codex, Gemini, Copilot) installed on the local machine.
package agent

import (
	"fmt"
	"os/exec"
)

// ===========================================
// Agent Registry
// ===========================================

// agentEntry describes a coding agent CLI binary and how to invoke it.
type agentEntry struct {
	Name   string
	Binary string
	Args   func(prompt string) []string
}

// agents lists coding agents in priority order.
var agents = []agentEntry{
	{
		Name:   "Claude Code",
		Binary: "claude",
		Args:   func(prompt string) []string { return []string{prompt} },
	},
	{
		Name:   "Codex",
		Binary: "codex",
		Args:   func(prompt string) []string { return []string{prompt} },
	},
	{
		Name:   "Gemini",
		Binary: "gemini",
		Args:   func(prompt string) []string { return []string{prompt} },
	},
	{
		Name:   "Copilot",
		Binary: "copilot",
		Args:   func(prompt string) []string { return []string{prompt} },
	},
}

// ===========================================
// Public API
// ===========================================

// FindAgent returns the name and binary of the first available coding agent
// on PATH. Returns an error if none are found.
func FindAgent() (name string, binary string, err error) {
	for _, a := range agents {
		if _, lookErr := exec.LookPath(a.Binary); lookErr == nil {
			return a.Name, a.Binary, nil
		}
	}
	return "", "", fmt.Errorf("no coding agent found on PATH (tried: claude, codex, gemini, copilot)")
}

// CommandWithPrompt returns an unstarted *exec.Cmd that invokes the first
// available coding agent with the given prompt.
// The caller is responsible for attaching I/O (e.g. via tea.ExecProcess).
func CommandWithPrompt(prompt string) (*exec.Cmd, error) {
	agent, err := findAgentEntry()
	if err != nil {
		return nil, err
	}
	return exec.Command(agent.Binary, agent.Args(prompt)...), nil
}

// ===========================================
// Helpers
// ===========================================

// findAgentEntry returns the first available agent entry.
func findAgentEntry() (*agentEntry, error) {
	for i := range agents {
		if _, err := exec.LookPath(agents[i].Binary); err == nil {
			return &agents[i], nil
		}
	}
	return nil, fmt.Errorf("no coding agent found on PATH (tried: claude, codex, gemini, copilot)")
}
