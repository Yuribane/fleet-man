package devcontainer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Client wraps the devcontainer CLI.
type Client struct {
	// Verbose sends devcontainer stderr to os.Stderr when true (for CLI use).
	Verbose bool
}

func NewClient() *Client {
	return &Client{}
}

// Up runs `devcontainer up` for the given workspace folder.
func (c *Client) Up(workspaceDir string) (*UpResult, error) {
	cmd := exec.Command("devcontainer", "up", "--workspace-folder", workspaceDir)
	if c.Verbose {
		cmd.Stderr = os.Stderr
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("devcontainer up failed: %w", err)
	}

	var result UpResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer up output: %w\nraw: %s", err, out)
	}

	if result.Outcome != "success" {
		return nil, fmt.Errorf("devcontainer up returned outcome %q", result.Outcome)
	}

	return &result, nil
}

// Exec runs `devcontainer exec` in the given workspace folder.
func (c *Client) Exec(workspaceDir string, command []string) error {
	args := append([]string{"exec", "--workspace-folder", workspaceDir}, command...)
	cmd := exec.Command("devcontainer", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Down stops and removes a container by ID using docker directly.
// The devcontainer CLI does not have a built-in down/stop command.
func (c *Client) Down(containerID string) error {
	cmd := exec.Command("docker", "rm", "-f", containerID)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops a container by ID without removing it.
func (c *Client) Stop(containerID string) error {
	cmd := exec.Command("docker", "stop", containerID)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Start starts an existing container by ID.
func (c *Client) Start(containerID string) error {
	cmd := exec.Command("docker", "start", containerID)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stats returns CPU and memory usage for the given container IDs.
// Uses `docker stats --no-stream` for a single snapshot.
func (c *Client) Stats(containerIDs []string) (map[string]*ContainerStats, error) {
	if len(containerIDs) == 0 {
		return nil, nil
	}

	args := append([]string{"stats", "--no-stream", "--format", "{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}"}, containerIDs...)
	cmd := exec.Command("docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*ContainerStats)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}

		id := fields[0]

		// CPU: "12.34%" → 123.4 millicores
		cpuStr := strings.TrimSuffix(strings.TrimSpace(fields[1]), "%")
		cpuPct, _ := strconv.ParseFloat(cpuStr, 64)
		cpuMillicores := cpuPct * 10 // 1% = 10 millicores

		// Memory: "123.4MiB / 1.5GiB" → take the usage part
		memParts := strings.SplitN(fields[2], "/", 2)
		memMB := parseMemToMB(strings.TrimSpace(memParts[0]))

		// Match by prefix — docker stats may return full or short IDs
		for _, cid := range containerIDs {
			if strings.HasPrefix(cid, id) || strings.HasPrefix(id, cid) {
				result[cid] = &ContainerStats{
					CPUMillicores: cpuMillicores,
					MemoryMB:      memMB,
				}
			}
		}
	}

	return result, nil
}

func parseMemToMB(s string) float64 {
	s = strings.TrimSpace(s)
	multiplier := 1.0

	if strings.HasSuffix(s, "GiB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "GiB")
	} else if strings.HasSuffix(s, "MiB") {
		multiplier = 1
		s = strings.TrimSuffix(s, "MiB")
	} else if strings.HasSuffix(s, "KiB") {
		multiplier = 1.0 / 1024
		s = strings.TrimSuffix(s, "KiB")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1.0 / (1024 * 1024)
		s = strings.TrimSuffix(s, "B")
	}

	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return val * multiplier
}

// Logs streams docker logs for a container.
func (c *Client) Logs(containerID string, follow bool) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, containerID)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
