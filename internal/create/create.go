package create

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	devcontainerbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

// Run performs git clone + devcontainer up for an instance that already exists
// in state.json with StatusCreating. On success it updates the instance to
// StatusRunning. On failure it sets StatusFailed with the error message.
func Run(fleetName, instanceName, remoteURL string, verbose bool) error {
	wsDir := filepath.Join(state.WorkspacesDir(), fleetName, instanceName, fleetName)

	if err := os.MkdirAll(filepath.Dir(wsDir), 0755); err != nil {
		setFailed(fleetName, instanceName, err)
		return fmt.Errorf("mkdir: %w", err)
	}

	gitClone := exec.Command("git", "clone", remoteURL, wsDir)
	if verbose {
		gitClone.Stdout = os.Stdout
		gitClone.Stderr = os.Stderr
		if err := gitClone.Run(); err != nil {
			wrapped := fmt.Errorf("git clone failed: %w", err)
			setFailed(fleetName, instanceName, wrapped)
			return wrapped
		}
	} else {
		if out, err := gitClone.CombinedOutput(); err != nil {
			wrapped := fmt.Errorf("git clone: %w\n%s", err, out)
			setFailed(fleetName, instanceName, wrapped)
			return wrapped
		}
	}

	dc := devcontainerbackend.New(devcontainerbackend.WithVerbose(verbose))
	result, err := dc.Up(wsDir)
	if err != nil {
		setFailed(fleetName, instanceName, err)
		return err
	}

	// Success: update state
	st, err := state.Load()
	if err != nil {
		return err
	}
	if f, ok := st.Fleets[fleetName]; ok {
		if inst, err := f.GetInstance(instanceName); err == nil {
			inst.ContainerID = result.ContainerID
			inst.Status = fleet.StatusRunning
			inst.Error = ""
		}
	}
	return state.Save(st)
}

func setFailed(fleetName, instanceName string, origErr error) {
	st, err := state.Load()
	if err != nil {
		return
	}
	if f, ok := st.Fleets[fleetName]; ok {
		if inst, err := f.GetInstance(instanceName); err == nil {
			inst.Status = fleet.StatusFailed
			inst.Error = origErr.Error()
		}
	}
	_ = state.Save(st)
}
