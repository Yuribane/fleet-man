package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Messages

type execDoneMsg struct{ err error }

type instanceSpawnedMsg struct {
	fleet    string
	instance string
}

type instanceCreateErrMsg struct {
	fleet    string
	instance string
	err      error
}

type pollCreatingTickMsg struct{}

type statsMsg struct {
	stats       map[string]*devcontainer.ContainerStats
	agentProbes map[string]int64 // containerID → CPU ticks (-1 = not found)
}

// Commands

func pollCreatingCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return pollCreatingTickMsg{}
	})
}

func fetchStatsCmd(ids []string, agentPattern string, delay bool) tea.Cmd {
	return func() tea.Msg {
		if delay {
			time.Sleep(3 * time.Second)
		}
		if len(ids) == 0 {
			return statsMsg{}
		}
		dc := devcontainer.NewClient()
		stats, _ := dc.Stats(ids)

		var agentProbes map[string]int64
		if agentPattern != "" {
			agentProbes = dc.AgentProbes(ids, agentPattern)
		}

		return statsMsg{stats: stats, agentProbes: agentProbes}
	}
}

// agentToolPattern returns the process substring to search for based
// on the configured agent tool. The AgentTool values ("claude", "codex",
// etc.) are already the process name substrings.
func agentToolPattern(cfg *state.Config) string {
	if cfg == nil {
		return ""
	}
	return string(cfg.AgentSettings.ToolSelection)
}

func createInstanceCmd(fleetName, instanceName, remoteURL string) tea.Cmd {
	return func() tea.Msg {
		self, err := os.Executable()
		if err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, fmt.Errorf("os.Executable: %w", err)}
		}

		cmd := exec.Command(self, "_create-instance", fleetName, instanceName, remoteURL)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		// Log output for debugging
		logDir := filepath.Join(state.FleetDir(), "logs")
		_ = os.MkdirAll(logDir, 0755)
		logFile, err := os.Create(filepath.Join(logDir, fleetName+"-"+instanceName+".log"))
		if err == nil {
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}

		if err := cmd.Start(); err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, fmt.Errorf("spawn: %w", err)}
		}

		// Detach: do not call cmd.Wait(). The child runs independently.
		return instanceSpawnedMsg{fleetName, instanceName}
	}
}
