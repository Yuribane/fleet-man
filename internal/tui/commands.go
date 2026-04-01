package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	coderbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/coder"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
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
	stats        map[string]*backend.ContainerStats
	screens      map[string]backend.ScreenCapture
	probes       map[string]string // containerID → detected tool name (from ps aux)
	containerIDs []string          // containers that were probed (for staleness detection)
}

// Commands

func pollCreatingCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return pollCreatingTickMsg{}
	})
}

func fetchStatsCmd(dc backend.Backend, ids []string, sessions map[string]string, delay bool) tea.Cmd {
	return func() tea.Msg {
		if delay {
			time.Sleep(3 * time.Second)
		}
		if len(ids) == 0 {
			return statsMsg{}
		}
		stats, _ := dc.Stats(ids)
		screens := backend.CaptureScreens(dc, sessions)
		probes := backend.AgentToolProbes(dc, ids)
		return statsMsg{stats: stats, screens: screens, probes: probes, containerIDs: ids}
	}
}

// operationDoneMsg is sent when a background instance operation completes.
type operationDoneMsg struct {
	fleet    string
	instance string
	message  string
	err      error
}

// toggleInstanceCmd runs stop/start in the background.
func toggleInstanceCmd(fleetName, instanceName string) tea.Cmd {
	toggle := toggleInstanceStatus // capture for goroutine
	return func() tea.Msg {
		result, err := toggle(fleetName, instanceName)
		if err != nil {
			return operationDoneMsg{fleetName, instanceName, "", err}
		}
		key := fleetName + "/" + instanceName
		var msg string
		switch result.Status {
		case fleet.StatusStopped:
			msg = fmt.Sprintf("Stopped %s", key)
		case fleet.StatusRunning:
			msg = fmt.Sprintf("Started %s", key)
		default:
			msg = fmt.Sprintf("Instance %s is %s", key, result.Status)
		}
		return operationDoneMsg{fleetName, instanceName, msg, nil}
	}
}

type dotfilesAutoInstallDoneMsg struct {
	instance string
	err      error
}

func autoInstallDotfilesCmd(dc backend.Backend, workspaceDir, instanceKey string, cfg *state.Config) tea.Cmd {
	return func() tea.Msg {
		setup := dotfilesSetupScript(cfg)
		if setup == "" {
			return dotfilesAutoInstallDoneMsg{instance: instanceKey}
		}
		cmd := dc.ExecCommand(workspaceDir, []string{"sh", "-c", setup})
		return dotfilesAutoInstallDoneMsg{instance: instanceKey, err: cmd.Run()}
	}
}

// deleteInstanceCmd runs instance deletion in the background.
func deleteInstanceCmd(dc backend.Backend, fleetName, instanceName, containerID, wsDir string) tea.Cmd {
	return func() tea.Msg {
		_ = dc.Down(containerID)
		if wsDir != "" {
			_ = os.RemoveAll(wsDir)
		}
		st, err := state.Load()
		if err == nil {
			if f, ok := st.Fleets[fleetName]; ok {
				_ = f.RemoveInstance(instanceName)
				_ = state.Save(st)
			}
		}
		key := fleetName + "/" + instanceName
		return operationDoneMsg{fleetName, instanceName, fmt.Sprintf("Removed %s", key), nil}
	}
}

// deleteFleetCmd runs fleet destruction in the background.
func deleteFleetCmd(backends map[fleet.BackendType]backend.Backend, fleetName string, instances []*fleet.Instance) tea.Cmd {
	// Snapshot what we need — don't capture model
	type target struct {
		dc          backend.Backend
		containerID string
		wsDir       string
	}
	var targets []target
	for _, inst := range instances {
		bt := inst.Backend
		if bt == "" {
			bt = fleet.BackendDevcontainer
		}
		targets = append(targets, target{backends[bt], inst.ContainerID, inst.WorkspaceDir})
	}
	return func() tea.Msg {
		for _, t := range targets {
			if t.dc != nil {
				_ = t.dc.Down(t.containerID)
			}
			if t.wsDir != "" {
				_ = os.RemoveAll(t.wsDir)
			}
		}
		st, err := state.Load()
		if err == nil {
			delete(st.Fleets, fleetName)
			_ = state.Save(st)
		}
		return operationDoneMsg{fleetName, "", fmt.Sprintf("Removed fleet %s", fleetName), nil}
	}
}

// coderParamsFetchedMsg is sent when template parameter fetching completes.
type coderParamsFetchedMsg struct {
	params  []coderbackend.RichParameter
	presets []coderbackend.Preset
	err     error
}

// fetchCoderParamsCmd fetches template parameters and presets asynchronously.
func fetchCoderParamsCmd(templateName string) tea.Cmd {
	return func() tea.Msg {
		versionID, err := coderbackend.FetchActiveVersionID(templateName)
		if err != nil {
			return coderParamsFetchedMsg{err: err}
		}

		params, err := coderbackend.FetchRichParameters(versionID)
		if err != nil {
			return coderParamsFetchedMsg{err: err}
		}

		presets, _ := coderbackend.FetchPresets(versionID)
		return coderParamsFetchedMsg{params: params, presets: presets}
	}
}

func createInstanceCmd(fleetName, instanceName, remoteURL string, bt fleet.BackendType) tea.Cmd {
	return func() tea.Msg {
		self, err := os.Executable()
		if err != nil {
			return instanceCreateErrMsg{fleetName, instanceName, fmt.Errorf("os.Executable: %w", err)}
		}

		cmd := exec.Command(self, "_create-instance", fleetName, instanceName, remoteURL, string(bt))
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
