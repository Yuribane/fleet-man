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

	// userCache caches the non-root username per container to avoid
	// repeated docker exec calls on every poll cycle.
	userCache   map[string]string
	userCacheMu sync.Mutex
}

func NewClient() *Client {
	return &Client{userCache: make(map[string]string)}
}

// containerUser returns the non-root user inside the container, caching
// the result to avoid a docker exec round-trip on every poll.
func (c *Client) containerUser(containerID string) string {
	c.userCacheMu.Lock()
	if u, ok := c.userCache[containerID]; ok {
		c.userCacheMu.Unlock()
		return u
	}
	c.userCacheMu.Unlock()

	cmd := exec.Command("docker", "exec", containerID, "sh", "-c",
		`for d in /home/*/; do [ -d "$d" ] && stat -c %U "$d" 2>/dev/null && exit; done`)
	out, err := cmd.Output()
	u := ""
	if err == nil {
		u = strings.TrimSpace(string(out))
	}

	c.userCacheMu.Lock()
	c.userCache[containerID] = u
	c.userCacheMu.Unlock()
	return u
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

// AgentProbeResult holds the outcome of probing a container for agent
// processes. Tool is the detected tool name. State is "working" or
// "waiting" for file-based detection (claude, copilot). CPUTicks holds
// cumulative ticks for tick-based detection (codex, gemini).
type AgentProbeResult struct {
	Tool     string
	State    string // "working" or "waiting" (file-based); empty for tick-based
	CPUTicks int64  // only used when State is empty
}

// probeScript is the shell script run inside each container to detect
// which agent tool is running and whether it is working or waiting.
//
// Phase 1: Process scan finds which tool is running (works universally).
// Phase 2: Tool-specific state determination:
//   - Claude Code: newest JSONL transcript mtime in ~/.claude/projects/
//   - Copilot: events.jsonl event type + mtime in ~/.copilot/session-state/
//   - Codex: newest rollout JSONL mtime in ~/.codex/sessions/
//   - Gemini: CPU tick sum for delta comparison (no known session files)
const probeScript = `
tool=""
for t in claude copilot codex gemini; do
  pids=$(ps aux 2>/dev/null | awk -v t="$t" '($11 ~ "(^|/)"t"$" || $12 ~ "(^|/)"t"$") {print $2}')
  [ -n "$pids" ] && { tool="$t"; break; }
done
[ -z "$tool" ] && { echo "- -"; exit 0; }
case "$tool" in
  claude)
    newest="" newest_mt=0
    for f in "$HOME"/.claude/projects/*/*.jsonl; do
      [ -f "$f" ] || continue
      mt=$(stat -c %Y "$f" 2>/dev/null)
      [ "${mt:-0}" -gt "$newest_mt" ] && { newest="$f"; newest_mt="$mt"; }
    done
    if [ -n "$newest" ]; then
      age=$(( $(date +%s) - newest_mt ))
      [ "$age" -lt 30 ] && echo "claude working" || echo "claude waiting"
    else
      echo "claude waiting"
    fi;;
  copilot)
    newest="" newest_mt=0
    for f in "$HOME"/.copilot/session-state/*/events.jsonl; do
      [ -f "$f" ] || continue
      mt=$(stat -c %Y "$f" 2>/dev/null)
      [ "${mt:-0}" -gt "$newest_mt" ] && { newest="$f"; newest_mt="$mt"; }
    done
    if [ -n "$newest" ]; then
      last=$(tail -1 "$newest" | sed -n 's/.*"type":"\([^"]*\)".*/\1/p')
      case "$last" in
        *turn_start*|*execution_start*|user.message) echo "copilot working";;
        *) age=$(( $(date +%s) - newest_mt ))
           [ "$age" -lt 30 ] && echo "copilot working" || echo "copilot waiting";;
      esac
    else
      echo "copilot waiting"
    fi;;
  codex)
    newest="" newest_mt=0
    for f in "$HOME"/.codex/sessions/*/*/*/*.jsonl "$HOME"/.codex/sessions/*/*.jsonl; do
      [ -f "$f" ] || continue
      mt=$(stat -c %Y "$f" 2>/dev/null)
      [ "${mt:-0}" -gt "$newest_mt" ] && { newest="$f"; newest_mt="$mt"; }
    done
    if [ -n "$newest" ]; then
      age=$(( $(date +%s) - newest_mt ))
      [ "$age" -lt 30 ] && echo "codex working" || echo "codex waiting"
    else
      echo "codex waiting"
    fi;;
  *)
    total=0
    for p in $pids; do
      v=$(awk '{printf "%d\n",$14+$15}' /proc/$p/stat 2>/dev/null)
      total=$((total + ${v:-0}))
    done
    echo "$tool $total";;
esac
`

// parseProbeOutput parses the probe script output.
// File-based tools return "tool working" or "tool waiting".
// Tick-based tools return "tool <ticks>".
// "- -" means no agent found (valid result, not a failure).
func parseProbeOutput(output string) (AgentProbeResult, bool) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) != 2 {
		return AgentProbeResult{}, false
	}
	if fields[0] == "-" {
		// No agent found — return empty result with found=true so the
		// caller knows the probe ran successfully (vs docker exec failure).
		return AgentProbeResult{}, true
	}
	switch fields[1] {
	case "working", "waiting":
		return AgentProbeResult{Tool: fields[0], State: fields[1]}, true
	}
	ticks, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return AgentProbeResult{}, false
	}
	return AgentProbeResult{Tool: fields[0], CPUTicks: ticks}, true
}

// AgentProbe runs the detection script inside a container to find which
// agent tool (if any) is running and whether it is working or waiting.
// Returns (result, true) when the probe ran (even if no agent found).
// Returns (_, false) only on docker exec failure (transient error).
func (c *Client) AgentProbe(containerID string) (AgentProbeResult, bool) {
	args := []string{"exec"}
	if u := c.containerUser(containerID); u != "" {
		args = append(args, "-u", u)
	}
	args = append(args, containerID, "sh", "-c", probeScript)
	cmd := exec.Command("docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return AgentProbeResult{}, false
	}
	return parseProbeOutput(string(out))
}

// AgentProbes runs AgentProbe concurrently for all containers.
// Containers whose probe succeeded are always in the result (even if no
// agent was found). Containers whose probe failed (docker exec error)
// are omitted so the caller can preserve their previous state.
func (c *Client) AgentProbes(containerIDs []string) map[string]AgentProbeResult {
	result := make(map[string]AgentProbeResult, len(containerIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range containerIDs {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			r, ok := c.AgentProbe(cid)
			mu.Lock()
			if ok {
				result[cid] = r
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
