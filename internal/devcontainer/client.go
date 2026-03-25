package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// Client wraps the devcontainer CLI.
type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// Up runs `devcontainer up` for the given workspace folder.
func (c *Client) Up(workspaceDir string) (*UpResult, error) {
	cmd := exec.Command("devcontainer", "up", "--workspace-folder", workspaceDir)
	cmd.Stderr = os.Stderr

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
