package portforward

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
)

// Forward represents a single active port forward binding.
type Forward struct {
	LocalPort    int  `json:"local_port"`
	RemotePort   int  `json:"remote_port"`
	BrowserProxy bool `json:"browser_proxy"` // true when this forward serves a browser proxy
	cmd          *exec.Cmd    // running process (external fallback)
	listener     net.Listener // in-process TCP listener
	done         chan struct{} // closed when the in-process proxy stops
}

// Label returns a display string like "8080->80".
// Browser proxy forwards are labelled "PORT->proxy" to distinguish
// them from regular port forwards.
func (f Forward) Label() string {
	if f.BrowserProxy {
		return fmt.Sprintf("%d->proxy", f.LocalPort)
	}
	return fmt.Sprintf("%d->%d", f.LocalPort, f.RemotePort)
}

// CmdFactory builds an unstarted *exec.Cmd for a port forward.
type CmdFactory func(containerID string, localPort, remotePort int) *exec.Cmd

// ResolveFunc returns a directly-reachable hostname for a container.
// Returns ("", false) when the container is not directly reachable.
type ResolveFunc func(containerID string) (string, bool)

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

// ===========================================
// In-process TCP proxy
// ===========================================

// startProxy opens a TCP listener on localPort and proxies each accepted
// connection to hostname:remotePort. All goroutines stop when the
// listener is closed.
func startProxy(localPort int, hostname string, remotePort int) (*Forward, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", localPort))
	if err != nil {
		return nil, err
	}

	target := net.JoinHostPort(hostname, fmt.Sprintf("%d", remotePort))
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go proxy(conn, target)
		}
	}()

	return &Forward{
		LocalPort:  localPort,
		RemotePort: remotePort,
		listener:   ln,
		done:       done,
	}, nil
}

// proxy pipes data between a local connection and a remote target.
func proxy(local net.Conn, target string) {
	remote, err := net.Dial("tcp", target)
	if err != nil {
		local.Close()
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(remote, local)
		if tc, ok := remote.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		io.Copy(local, remote)
		if tc, ok := local.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
	local.Close()
	remote.Close()
}

// stopForward cleanly shuts down a forward, whether it uses an
// in-process listener or an external process.
func stopForward(f *Forward) {
	if f.listener != nil {
		f.listener.Close()
		if f.done != nil {
			<-f.done
		}
	}
	if f.cmd != nil && f.cmd.Process != nil {
		_ = f.cmd.Process.Kill()
	}
}
