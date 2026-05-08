package portforward

import "os/exec"

// CmdFactory builds an unstarted *exec.Cmd for a port forward.
type CmdFactory func(containerID string, localPort, remotePort int) *exec.Cmd

// ResolveFunc returns a directly-reachable hostname for a container.
// Returns ("", false) when the container is not directly reachable.
type ResolveFunc func(containerID string) (string, bool)
