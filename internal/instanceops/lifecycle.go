package instanceops

import (
	"fmt"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

type containerController interface {
	Start(containerID string) error
	Stop(containerID string) error
}

var newClient = func() containerController {
	return devcontainer.NewClient()
}

type Result struct {
	FleetName      string
	InstanceName   string
	PreviousStatus fleet.InstanceStatus
	Status         fleet.InstanceStatus
	Changed        bool
}

func StopInstance(fleetName, instanceName string) (*Result, error) {
	return transitionInstance(fleetName, instanceName, fleet.StatusStopped)
}

func StartInstance(fleetName, instanceName string) (*Result, error) {
	return transitionInstance(fleetName, instanceName, fleet.StatusRunning)
}

func ToggleInstance(fleetName, instanceName string) (*Result, error) {
	st, _, inst, err := loadInstance(fleetName, instanceName)
	if err != nil {
		return nil, err
	}

	switch inst.Status {
	case fleet.StatusRunning:
		return transitionLoadedInstance(st, inst, fleetName, instanceName, fleet.StatusStopped)
	case fleet.StatusStopped:
		return transitionLoadedInstance(st, inst, fleetName, instanceName, fleet.StatusRunning)
	default:
		return nil, fmt.Errorf("instance %s/%s cannot be toggled from status %q", fleetName, instanceName, inst.Status)
	}
}

func transitionInstance(fleetName, instanceName string, targetStatus fleet.InstanceStatus) (*Result, error) {
	st, _, inst, err := loadInstance(fleetName, instanceName)
	if err != nil {
		return nil, err
	}

	return transitionLoadedInstance(st, inst, fleetName, instanceName, targetStatus)
}

func transitionLoadedInstance(st *state.State, inst *fleet.Instance, fleetName, instanceName string, targetStatus fleet.InstanceStatus) (*Result, error) {
	result := &Result{
		FleetName:      fleetName,
		InstanceName:   instanceName,
		PreviousStatus: inst.Status,
		Status:         inst.Status,
	}

	if inst.Status == targetStatus {
		return result, nil
	}

	if inst.ContainerID == "" {
		return nil, fmt.Errorf("instance %s/%s has no container ID", fleetName, instanceName)
	}

	dc := newClient()

	switch targetStatus {
	case fleet.StatusStopped:
		if inst.Status != fleet.StatusRunning {
			return nil, fmt.Errorf("instance %s/%s cannot be stopped from status %q", fleetName, instanceName, inst.Status)
		}
		if err := dc.Stop(inst.ContainerID); err != nil {
			return nil, fmt.Errorf("stop instance %s/%s: %w", fleetName, instanceName, err)
		}
	case fleet.StatusRunning:
		if inst.Status != fleet.StatusStopped {
			return nil, fmt.Errorf("instance %s/%s cannot be started from status %q", fleetName, instanceName, inst.Status)
		}
		if err := dc.Start(inst.ContainerID); err != nil {
			return nil, fmt.Errorf("start instance %s/%s: %w", fleetName, instanceName, err)
		}
	default:
		return nil, fmt.Errorf("unsupported target status %q", targetStatus)
	}

	inst.Status = targetStatus
	if targetStatus == fleet.StatusRunning {
		inst.Error = ""
	}

	if err := state.Save(st); err != nil {
		return nil, err
	}

	result.Status = inst.Status
	result.Changed = true
	return result, nil
}

func loadInstance(fleetName, instanceName string) (*state.State, *fleet.Fleet, *fleet.Instance, error) {
	st, err := state.Load()
	if err != nil {
		return nil, nil, nil, err
	}

	f, ok := st.Fleets[fleetName]
	if !ok {
		return nil, nil, nil, fmt.Errorf("fleet %q not found", fleetName)
	}

	inst, err := f.GetInstance(instanceName)
	if err != nil {
		return nil, nil, nil, err
	}

	return st, f, inst, nil
}
