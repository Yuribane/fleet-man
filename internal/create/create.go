package create

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
	"github.com/BenjaminBenetti/fleet-man/internal/backendutil"
	coderbackend "github.com/BenjaminBenetti/fleet-man/internal/backend/coder"
	"github.com/BenjaminBenetti/fleet-man/internal/dotfiles"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

// Run performs instance creation for an instance that already exists
// in state.json with StatusCreating. For devcontainer backends it runs
// git clone + devcontainer up. For coder backends it runs coder create.
// On success it updates the instance to StatusRunning. On failure it sets
// StatusFailed with the error message.
func Run(fleetName, instanceName, remoteURL string, verbose bool, bt fleet.BackendType) error {
	wsDir := filepath.Join(state.WorkspacesDir(), fleetName, instanceName, fleetName)

	var dc backend.Backend
	if bt == fleet.BackendCoder {
		dc = buildCoderBackend(fleetName, instanceName, remoteURL, verbose)
	} else {
		dc = backendutil.New(bt, verbose)
	}

	if bt != fleet.BackendCoder {
		// Devcontainer: clone repo first, then provision
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
	}

	result, err := dc.Up(wsDir)
	if err != nil {
		setFailed(fleetName, instanceName, err)
		return err
	}

	// Auto-install dotfiles while still in "creating" state so the
	// instance does not appear "running" before dotfiles are ready.
	cfg, _ := state.LoadConfig()
	if cfg != nil && cfg.DotfilesSettings.AutoInstall {
		if script := dotfiles.SetupScript(cfg); script != "" {
			cmd := dc.ExecCommand(wsDir, []string{"sh", "-c", script})
			if err := cmd.Run(); err != nil {
				setFailed(fleetName, instanceName, fmt.Errorf("dotfiles install: %w", err))
				return fmt.Errorf("dotfiles install: %w", err)
			}
		}
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

// buildCoderBackend creates a CoderBackend configured from ~/.fleet/config.json
// with template, preset, and resolved parameter bindings.
func buildCoderBackend(fleetName, instanceName, remoteURL string, verbose bool) backend.Backend {
	opts := []coderbackend.Option{}
	if verbose {
		opts = append(opts, coderbackend.WithVerbose(true))
	}

	cfg, err := state.LoadConfig()
	if err != nil || cfg == nil {
		return coderbackend.New(opts...)
	}

	cs := cfg.CoderSettings
	if cs.Template != "" {
		opts = append(opts, coderbackend.WithTemplate(cs.Template))
	}
	if cs.Preset != "" {
		opts = append(opts, coderbackend.WithPreset(cs.Preset))
	}

	// Resolve parameters with variable substitution
	if len(cs.Parameters) > 0 {
		wsName := fleetName + "-" + instanceName
		resolved := make(map[string]string, len(cs.Parameters))
		for _, p := range cs.Parameters {
			val := p.Value
			if val == "" {
				val = p.DefaultValue
			}
			val = strings.ReplaceAll(val, "${GIT_URL}", remoteURL)
			val = strings.ReplaceAll(val, "${INSTANCE_NAME}", wsName)
			if val != "" {
				resolved[p.Name] = val
			}
		}
		opts = append(opts, coderbackend.WithParameters(resolved))
	}

	return coderbackend.New(opts...)
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
