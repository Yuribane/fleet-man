package portforward

import (
	"fmt"
	"os/exec"
	"sync"
)

// Forward represents a single active port forward binding.
type Forward struct {
	LocalPort  int       `json:"local_port"`
	RemotePort int       `json:"remote_port"`
	cmd        *exec.Cmd // running process (not serialized)
}

// Label returns a display string like "8080->80".
func (f Forward) Label() string {
	return fmt.Sprintf("%d->%d", f.LocalPort, f.RemotePort)
}

// CmdFactory builds an unstarted *exec.Cmd for a port forward.
type CmdFactory func(containerID string, localPort, remotePort int) *exec.Cmd

// Manager tracks active port forward processes per instance.
type Manager struct {
	mu       sync.Mutex
	forwards map[string][]*Forward // key: "fleet/instance"
}

// NewManager creates a new port forward manager.
func NewManager() *Manager {
	return &Manager{
		forwards: make(map[string][]*Forward),
	}
}

// Add starts a new port forward for the given instance. Returns an error
// if the local port is already in use by another forward on this instance.
func (m *Manager) Add(key string, localPort, remotePort int, factory CmdFactory, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, f := range m.forwards[key] {
		if f.LocalPort == localPort {
			return fmt.Errorf("local port %d is already forwarded on %s", localPort, key)
		}
	}

	cmd := factory(containerID, localPort, remotePort)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start port forward %d->%d: %w", localPort, remotePort, err)
	}

	fwd := &Forward{
		LocalPort:  localPort,
		RemotePort: remotePort,
		cmd:        cmd,
	}
	m.forwards[key] = append(m.forwards[key], fwd)

	// Reap the process in the background so it doesn't become a zombie.
	go cmd.Wait() //nolint:errcheck

	return nil
}

// Remove stops and removes the port forward on the given local port.
func (m *Manager) Remove(key string, localPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fwds := m.forwards[key]
	for i, f := range fwds {
		if f.LocalPort == localPort {
			if f.cmd != nil && f.cmd.Process != nil {
				_ = f.cmd.Process.Kill()
			}
			m.forwards[key] = append(fwds[:i], fwds[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no forward on local port %d for %s", localPort, key)
}

// RemoveAll stops all port forwards for an instance.
func (m *Manager) RemoveAll(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, f := range m.forwards[key] {
		if f.cmd != nil && f.cmd.Process != nil {
			_ = f.cmd.Process.Kill()
		}
	}
	delete(m.forwards, key)
}

// List returns a copy of active forwards for the given instance.
func (m *Manager) List(key string) []Forward {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	fwds := m.forwards[key]
	result := make([]Forward, len(fwds))
	for i, f := range fwds {
		result[i] = Forward{LocalPort: f.LocalPort, RemotePort: f.RemotePort}
	}
	return result
}

// ListAll returns all active forwards grouped by instance key.
func (m *Manager) ListAll() map[string][]Forward {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string][]Forward, len(m.forwards))
	for key, fwds := range m.forwards {
		entries := make([]Forward, len(fwds))
		for i, f := range fwds {
			entries[i] = Forward{LocalPort: f.LocalPort, RemotePort: f.RemotePort}
		}
		result[key] = entries
	}
	return result
}

// FormatLabels returns a comma-separated string of forward labels for an instance.
// Returns empty string if no forwards are active. Safe to call on a nil manager.
func (m *Manager) FormatLabels(key string) string {
	if m == nil {
		return ""
	}
	fwds := m.List(key)
	if len(fwds) == 0 {
		return ""
	}
	labels := ""
	for i, f := range fwds {
		if i > 0 {
			labels += ", "
		}
		labels += f.Label()
	}
	return labels
}

// Shutdown stops all active port forwards. Safe to call on a nil manager.
func (m *Manager) Shutdown() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, fwds := range m.forwards {
		for _, f := range fwds {
			if f.cmd != nil && f.cmd.Process != nil {
				_ = f.cmd.Process.Kill()
			}
		}
		delete(m.forwards, key)
	}
}
