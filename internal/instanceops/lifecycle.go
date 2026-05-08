package instanceops

import (
	"fmt"

	"github.com/BenjaminBenetti/fleet-man/internal/backendutil"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

type containerController interface {
	Start(containerID string) error
	Stop(containerID string) error
}

var newClient = func(backendType fleet.BackendType) containerController {
	return backendutil.New(backendType, false)
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
	st, _, instance, err := loadInstance(fleetName, instanceName)
	if err != nil {
		return nil, err
	}

	switch instance.Status {
	case fleet.StatusRunning, fleet.StatusStopping:
		return transitionLoadedInstance(st, instance, fleetName, instanceName, fleet.StatusStopped)
	case fleet.StatusStopped, fleet.StatusStarting:
		return transitionLoadedInstance(st, instance, fleetName, instanceName, fleet.StatusRunning)
	default:
		return nil, fmt.Errorf("instance %s/%s cannot be toggled from status %q", fleetName, instanceName, instance.Status)
	}
}

func transitionInstance(fleetName, instanceName string, targetStatus fleet.InstanceStatus) (*Result, error) {
	st, _, instance, err := loadInstance(fleetName, instanceName)
	if err != nil {
		return nil, err
	}

	return transitionLoadedInstance(st, instance, fleetName, instanceName, targetStatus)
}

func transitionLoadedInstance(st *state.State, instance *fleet.Instance, fleetName, instanceName string, targetStatus fleet.InstanceStatus) (*Result, error) {
	result := &Result{
		FleetName:      fleetName,
		InstanceName:   instanceName,
		PreviousStatus: instance.Status,
		Status:         instance.Status,
	}

	if instance.Status == targetStatus {
		return result, nil
	}

	if instance.ContainerID == "" {
		return nil, fmt.Errorf("instance %s/%s has no container ID", fleetName, instanceName)
	}

	instanceBackend := newClient(instance.Backend)

	switch targetStatus {
	case fleet.StatusStopped:
		if instance.Status != fleet.StatusRunning && instance.Status != fleet.StatusStopping {
			return nil, fmt.Errorf("instance %s/%s cannot be stopped from status %q", fleetName, instanceName, instance.Status)
		}
		if err := instanceBackend.Stop(instance.ContainerID); err != nil {
			return nil, fmt.Errorf("stop instance %s/%s: %w", fleetName, instanceName, err)
		}
	case fleet.StatusRunning:
		if instance.Status != fleet.StatusStopped && instance.Status != fleet.StatusStarting {
			return nil, fmt.Errorf("instance %s/%s cannot be started from status %q", fleetName, instanceName, instance.Status)
		}
		if err := instanceBackend.Start(instance.ContainerID); err != nil {
			return nil, fmt.Errorf("start instance %s/%s: %w", fleetName, instanceName, err)
		}
	default:
		return nil, fmt.Errorf("unsupported target status %q", targetStatus)
	}

	instance.Status = targetStatus
	if targetStatus == fleet.StatusRunning {
		instance.Error = ""
	}

	if err := state.Save(st); err != nil {
		return nil, err
	}

	result.Status = instance.Status
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

	instance, err := f.GetInstance(instanceName)
	if err != nil {
		return nil, nil, nil, err
	}

	return st, f, instance, nil
}
