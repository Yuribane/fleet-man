package doctor

import (
	"fmt"
	"os"
	"os/exec"
)

// ===========================================
// Constants
// ===========================================

const doctorURL = "https://raw.githubusercontent.com/BenjaminBenetti/fleet-man/main/skills/DOCTOR.md"

// DoctorPrompt is the instruction sent to the coding agent.
var DoctorPrompt = fmt.Sprintf("Read %s and follow the instructions", doctorURL)

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

// Command returns an unstarted *exec.Cmd for the doctor session.
// The caller is responsible for attaching I/O (e.g. via tea.ExecProcess).
func Command() (*exec.Cmd, error) {
	agent, err := findAgentEntry()
	if err != nil {
		return nil, err
	}
	return exec.Command(agent.Binary, agent.Args(DoctorPrompt)...), nil
}

// Run finds the first available coding agent and launches it interactively
// with the doctor prompt. Blocks until the agent exits.
func Run() error {
	cmd, err := Command()
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
