package portforward

import (
	"fmt"
	"net"
	"sync"
)

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

// Add starts a new port forward for the given instance. If resolve is
// non-nil and returns a reachable hostname, an in-process TCP proxy is
// used. Otherwise it falls back to spawning a process via factory.
func (m *Manager) Add(key string, localPort, remotePort int, factory CmdFactory, containerID string, resolve ResolveFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, f := range m.forwards[key] {
		if f.LocalPort == localPort {
			return fmt.Errorf("local port %d is already forwarded on %s", localPort, key)
		}
	}

	// Try in-process proxy via ResolveHostname.
	if resolve != nil {
		if hostname, ok := resolve(containerID); ok {
			fwd, err := startProxy(localPort, hostname, remotePort)
			if err != nil {
				return fmt.Errorf("start proxy %d->%d: %w", localPort, remotePort, err)
			}
			m.forwards[key] = append(m.forwards[key], fwd)
			return nil
		}
	}

	// Fallback: spawn an external process.
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

// FindBrowserProxy returns the local port of an existing browser proxy
// forward for the given instance key. Returns (0, false) if none exists.
func (m *Manager) FindBrowserProxy(key string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, f := range m.forwards[key] {
		if f.BrowserProxy {
			return f.LocalPort, true
		}
	}
	return 0, false
}

// AddBrowserProxy creates a port forward marked as a browser proxy,
// automatically selecting an available local port. It returns the
// chosen local port on success.
func (m *Manager) AddBrowserProxy(key string, remotePort int, factory CmdFactory, containerID string, resolve ResolveFunc) (int, error) {
	// Grab an ephemeral port from the OS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find available port: %w", err)
	}
	localPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Try in-process proxy via ResolveHostname.
	if resolve != nil {
		if hostname, ok := resolve(containerID); ok {
			fwd, err := startProxy(localPort, hostname, remotePort)
			if err != nil {
				return 0, fmt.Errorf("start proxy %d->%d: %w", localPort, remotePort, err)
			}
			fwd.BrowserProxy = true
			m.forwards[key] = append(m.forwards[key], fwd)
			return localPort, nil
		}
	}

	// Fallback: spawn an external process.
	cmd := factory(containerID, localPort, remotePort)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start port forward %d->%d: %w", localPort, remotePort, err)
	}

	fwd := &Forward{
		LocalPort:    localPort,
		RemotePort:   remotePort,
		BrowserProxy: true,
		cmd:          cmd,
	}
	m.forwards[key] = append(m.forwards[key], fwd)

	// Reap the process in the background so it doesn't become a zombie.
	go cmd.Wait() //nolint:errcheck

	return localPort, nil
}

// Remove stops and removes the port forward on the given local port.
func (m *Manager) Remove(key string, localPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fwds := m.forwards[key]
	for i, f := range fwds {
		if f.LocalPort == localPort {
			stopForward(f)
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
		stopForward(f)
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
		result[i] = Forward{LocalPort: f.LocalPort, RemotePort: f.RemotePort, BrowserProxy: f.BrowserProxy}
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
			entries[i] = Forward{LocalPort: f.LocalPort, RemotePort: f.RemotePort, BrowserProxy: f.BrowserProxy}
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
			stopForward(f)
		}
		delete(m.forwards, key)
	}
}
