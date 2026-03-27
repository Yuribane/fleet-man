package devcontainer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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
	args := []string{"up", "--workspace-folder", workspaceDir}
	args = append(args, sshUpArgs()...)
	cmd := exec.Command("devcontainer", args...)
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
	args := ExecArgs(workspaceDir, command)
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

// binaryMatchPattern builds an awk regex that matches a binary name in
// the command ($11) or first argument ($12) of ps aux output. Checks
// both fields to handle direct invocations ("claude --flag") and
// node-launched tools ("node /path/to/bin/copilot"). Does not match
// occurrences buried deeper in arguments (e.g. sshfs mounts containing
// ".claude").
func binaryMatchPattern(s string) string {
	if len(s) == 0 {
		return s
	}
	return `(^|\/)` + s + `$`
}

// AgentProbeResult holds the outcome of probing a container for agent
// processes. Tool is the detected tool name; CPUTicks is the cumulative
// CPU ticks (-1 when no agent was found).
type AgentProbeResult struct {
	Tool     string
	CPUTicks int64
}

// parseProbeOutput parses "tool ticks" output from the probe script.
func parseProbeOutput(output string) (AgentProbeResult, bool) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 || fields[0] == "-" {
		return AgentProbeResult{}, false
	}
	ticks, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return AgentProbeResult{}, false
	}
	return AgentProbeResult{Tool: fields[0], CPUTicks: ticks}, true
}

// AgentProbe checks if any of the given tool processes are running inside
// the container and returns which tool was found along with its cumulative
// CPU ticks (utime+stime from /proc/pid/stat).
//
// Uses `docker exec` with a single ps+awk pass to scan for all tools.
// Matches the command ($11) or first arg ($12) to handle both direct
// binaries and node-launched tools.
func (c *Client) AgentProbe(containerID string, tools []string) (AgentProbeResult, bool) {
	if len(tools) == 0 {
		return AgentProbeResult{}, false
	}

	var awkRules []string
	for _, t := range tools {
		pat := binaryMatchPattern(t)
		awkRules = append(awkRules, fmt.Sprintf(
			`($11 ~ /%s/ || $12 ~ /%s/) {print "%s",$2; exit}`,
			pat, pat, t,
		))
	}
	awkProgram := strings.Join(awkRules, " ")

	script := fmt.Sprintf(`info=$(ps aux 2>/dev/null | awk '%s')`, awkProgram) +
		`; [ -z "$info" ] && echo "- -1" || { tool=${info%% *}; pid=${info##* }; t=$(awk '{printf "%d\n",$14+$15}' /proc/$pid/stat 2>/dev/null || echo -1); echo "$tool $t"; }`

	cmd := exec.Command("docker", "exec", containerID, "sh", "-c", script)
	out, err := cmd.Output()
	if err != nil {
		return AgentProbeResult{}, false
	}
	return parseProbeOutput(string(out))
}

// AgentProbes runs AgentProbe concurrently for all containers.
func (c *Client) AgentProbes(containerIDs []string, tools []string) map[string]AgentProbeResult {
	result := make(map[string]AgentProbeResult, len(containerIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range containerIDs {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			r, found := c.AgentProbe(cid, tools)
			mu.Lock()
			if found {
				result[cid] = r
			} else {
				result[cid] = AgentProbeResult{CPUTicks: -1}
			}
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return result
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
