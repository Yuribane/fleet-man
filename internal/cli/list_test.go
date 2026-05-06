package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

func TestListIncludesBranchColumnAndValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{
		Fleets: map[string]*fleet.Fleet{
			"alpha": {
				Name: "alpha",
				Instances: []*fleet.Instance{
					{
						Name:         "agent-1",
						ContainerID:  "abc123456789999",
						WorkspaceDir: "/workspace/alpha/agent-1",
						CreatedAt:    time.Unix(0, 0),
						Status:       fleet.StatusRunning,
					},
					{
						Name:         "agent-2",
						ContainerID:  "def987654321000",
						WorkspaceDir: "/workspace/alpha/agent-2",
						CreatedAt:    time.Unix(0, 0),
						Status:       fleet.StatusStopped,
					},
				},
			},
		},
	}
	if err := state.Save(st); err != nil {
		t.Fatalf("state.Save() error = %v", err)
	}

	var out bytes.Buffer
	prevOutput := listOutput
	listOutput = &out
	defer func() { listOutput = prevOutput }()

	prevBranchName := listBranchName
	listBranchName = func(workspaceDir string) string {
		if workspaceDir == "/workspace/alpha/agent-1" {
			return "feature/status-line"
		}
		return ""
	}
	defer func() { listBranchName = prevBranchName }()

	cmd := newListCmd()
	if err := cmd.RunE(cmd, []string{"alpha"}); err != nil {
		t.Fatalf("RunE() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "BRANCH") {
		t.Fatalf("output missing BRANCH header:\n%s", output)
	}
	if !strings.Contains(output, "feature/status-line") {
		t.Fatalf("output missing branch value:\n%s", output)
	}

	agentTwoLine := findLine(output, "agent-2")
	if agentTwoLine == "" {
		t.Fatalf("output missing agent-2 line:\n%s", output)
	}
	if strings.Contains(agentTwoLine, "feature/status-line") {
		t.Fatalf("agent-2 line unexpectedly contains branch value:\n%s", agentTwoLine)
	}
	expectedCreated := time.Unix(0, 0).Local().Format("2006-01-02 15:04")
	if !strings.HasSuffix(strings.TrimRight(agentTwoLine, " "), expectedCreated) {
		t.Fatalf("agent-2 line should end at CREATED column when branch is empty:\n%s", agentTwoLine)
	}
}

func findLine(output, needle string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
