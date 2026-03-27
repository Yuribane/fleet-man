package tui

import (
	"strings"
	"testing"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/fleet"
	"github.com/BenjaminBenetti/fleet-man/internal/instanceops"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateNormalStopShortcutStopsRunningInstance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	inst := &fleet.Instance{Name: "agent-1", Status: fleet.StatusRunning}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		rows: []row{{kind: rowInstance, fleetName: "alpha", instance: inst}},
	}

	calledFleet := ""
	calledInstance := ""
	restore := stubToggleInstance(func(fleetName, instanceName string) (*instanceops.Result, error) {
		calledFleet = fleetName
		calledInstance = instanceName
		return &instanceops.Result{
			FleetName:    fleetName,
			InstanceName: instanceName,
			Status:       fleet.StatusStopped,
			Changed:      true,
		}, nil
	})
	defer restore()

	updated, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := updated.(model)

	if calledFleet != "alpha" || calledInstance != "agent-1" {
		t.Fatalf("toggle called with %s/%s, want alpha/agent-1", calledFleet, calledInstance)
	}
	if got.message != "Stopped alpha/agent-1" {
		t.Fatalf("message = %q, want %q", got.message, "Stopped alpha/agent-1")
	}
}

func TestUpdateNormalStopShortcutStartsStoppedInstance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	inst := &fleet.Instance{Name: "agent-1", Status: fleet.StatusStopped}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		rows: []row{{kind: rowInstance, fleetName: "alpha", instance: inst}},
	}

	restore := stubToggleInstance(func(fleetName, instanceName string) (*instanceops.Result, error) {
		return &instanceops.Result{
			FleetName:    fleetName,
			InstanceName: instanceName,
			Status:       fleet.StatusRunning,
			Changed:      true,
		}, nil
	})
	defer restore()

	updated, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := updated.(model)

	if got.message != "Started alpha/agent-1" {
		t.Fatalf("message = %q, want %q", got.message, "Started alpha/agent-1")
	}
}

func TestUpdateNormalStopShortcutRequiresInstanceRow(t *testing.T) {
	m := model{
		rows: []row{{kind: rowFleetHeader, fleetName: "alpha"}},
	}

	called := false
	restore := stubToggleInstance(func(fleetName, instanceName string) (*instanceops.Result, error) {
		called = true
		return nil, nil
	})
	defer restore()

	updated, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := updated.(model)

	if called {
		t.Fatal("toggle should not be called for a fleet header row")
	}
	if got.message != "Select an instance" {
		t.Fatalf("message = %q, want %q", got.message, "Select an instance")
	}
}

func TestUpdateNormalStopShortcutSkipsCreatingInstance(t *testing.T) {
	inst := &fleet.Instance{Name: "agent-1", Status: fleet.StatusCreating}
	m := model{
		rows: []row{{kind: rowInstance, fleetName: "alpha", instance: inst}},
	}

	called := false
	restore := stubToggleInstance(func(fleetName, instanceName string) (*instanceops.Result, error) {
		called = true
		return nil, nil
	})
	defer restore()

	updated, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := updated.(model)

	if called {
		t.Fatal("toggle should not be called for a creating instance")
	}
	if got.message != "Instance alpha/agent-1 is still creating" {
		t.Fatalf("message = %q, want %q", got.message, "Instance alpha/agent-1 is still creating")
	}
}

func TestViewFleetListShowsBranchItemForInstance(t *testing.T) {
	inst := &fleet.Instance{
		Name:         "agent-1",
		Status:       fleet.StatusRunning,
		WorkspaceDir: "/workspace/alpha/agent-1",
	}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha", instance: inst},
			{kind: rowSettings},
		},
	}

	prevResolveBranch := resolveWorkspaceBranch
	resolveWorkspaceBranch = func(workspaceDir string) string {
		if workspaceDir == "/workspace/alpha/agent-1" {
			return "feature/status-line"
		}
		return ""
	}
	defer func() { resolveWorkspaceBranch = prevResolveBranch }()

	view := m.viewFleetList()
	if !strings.Contains(view, "feature/status-line") {
		t.Fatalf("view missing branch item:\n%s", view)
	}
}

func TestViewFleetListOmitsBranchItemWhenBranchIsUnavailable(t *testing.T) {
	inst := &fleet.Instance{
		Name:         "agent-1",
		Status:       fleet.StatusRunning,
		WorkspaceDir: "/workspace/alpha/agent-1",
	}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha", instance: inst},
			{kind: rowSettings},
		},
	}

	prevResolveBranch := resolveWorkspaceBranch
	resolveWorkspaceBranch = func(string) string { return "" }
	defer func() { resolveWorkspaceBranch = prevResolveBranch }()

	view := m.viewFleetList()
	if strings.Contains(view, "feature/status-line") {
		t.Fatalf("view unexpectedly contains branch item:\n%s", view)
	}
}

func TestViewFleetListShowsAgentWorkingIndicator(t *testing.T) {
	inst := &fleet.Instance{
		Name:         "agent-1",
		Status:       fleet.StatusRunning,
		ContainerID:  "abc123",
		WorkspaceDir: "/workspace/alpha/agent-1",
	}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		cfg: &state.Config{
			AgentSettings: state.AgentSettings{ToolSelection: state.AgentToolClaude},
		},
		agentStates:    map[string]agentState{"abc123": agentWorking},
		agentPrevTicks: map[string]int64{},
		stats:          map[string]*devcontainer.ContainerStats{},
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha", instance: inst},
			{kind: rowSettings},
		},
	}

	prevResolveBranch := resolveWorkspaceBranch
	resolveWorkspaceBranch = func(string) string { return "" }
	defer func() { resolveWorkspaceBranch = prevResolveBranch }()

	view := m.viewFleetList()
	if !strings.Contains(view, "\u25b6") || !strings.Contains(view, "Claude Code") {
		t.Fatalf("view missing working indicator:\n%s", view)
	}
}

func TestViewFleetListShowsAgentWaitingIndicator(t *testing.T) {
	inst := &fleet.Instance{
		Name:         "agent-1",
		Status:       fleet.StatusRunning,
		ContainerID:  "abc123",
		WorkspaceDir: "/workspace/alpha/agent-1",
	}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		cfg: &state.Config{
			AgentSettings: state.AgentSettings{ToolSelection: state.AgentToolClaude},
		},
		agentStates:    map[string]agentState{"abc123": agentWaiting},
		agentPrevTicks: map[string]int64{},
		stats:          map[string]*devcontainer.ContainerStats{},
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha", instance: inst},
			{kind: rowSettings},
		},
	}

	prevResolveBranch := resolveWorkspaceBranch
	resolveWorkspaceBranch = func(string) string { return "" }
	defer func() { resolveWorkspaceBranch = prevResolveBranch }()

	view := m.viewFleetList()
	if !strings.Contains(view, "\u23f8") || !strings.Contains(view, "Claude Code") {
		t.Fatalf("view missing waiting indicator:\n%s", view)
	}
}

func TestViewFleetListShowsAgentOffIndicator(t *testing.T) {
	inst := &fleet.Instance{
		Name:         "agent-1",
		Status:       fleet.StatusRunning,
		ContainerID:  "abc123",
		WorkspaceDir: "/workspace/alpha/agent-1",
	}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		cfg: &state.Config{
			AgentSettings: state.AgentSettings{ToolSelection: state.AgentToolClaude},
		},
		agentStates:    map[string]agentState{"abc123": agentNotRunning},
		agentPrevTicks: map[string]int64{},
		stats:          map[string]*devcontainer.ContainerStats{},
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha", instance: inst},
			{kind: rowSettings},
		},
	}

	prevResolveBranch := resolveWorkspaceBranch
	resolveWorkspaceBranch = func(string) string { return "" }
	defer func() { resolveWorkspaceBranch = prevResolveBranch }()

	view := m.viewFleetList()
	if !strings.Contains(view, "idle") {
		t.Fatalf("view missing off/idle indicator:\n%s", view)
	}
	if strings.Contains(view, "\u25b6") || strings.Contains(view, "\u23f8") {
		t.Fatalf("not-running instance should not show working/waiting icon:\n%s", view)
	}
}

func TestViewFleetListNoAgentIndicatorForStoppedInstance(t *testing.T) {
	inst := &fleet.Instance{
		Name:         "agent-1",
		Status:       fleet.StatusStopped,
		ContainerID:  "abc123",
		WorkspaceDir: "/workspace/alpha/agent-1",
	}
	m := model{
		st: &state.State{
			Fleets: map[string]*fleet.Fleet{
				"alpha": {Name: "alpha", Instances: []*fleet.Instance{inst}},
			},
		},
		cfg: &state.Config{
			AgentSettings: state.AgentSettings{ToolSelection: state.AgentToolClaude},
		},
		agentStates:    map[string]agentState{},
		agentPrevTicks: map[string]int64{},
		stats:          map[string]*devcontainer.ContainerStats{},
		rows: []row{
			{kind: rowFleetHeader, fleetName: "alpha"},
			{kind: rowInstance, fleetName: "alpha", instance: inst},
			{kind: rowSettings},
		},
	}

	prevResolveBranch := resolveWorkspaceBranch
	resolveWorkspaceBranch = func(string) string { return "" }
	defer func() { resolveWorkspaceBranch = prevResolveBranch }()

	view := m.viewFleetList()
	if strings.Contains(view, "Claude Code") || strings.Contains(view, "idle") {
		t.Fatalf("stopped instance should not have agent indicator:\n%s", view)
	}
}

func stubToggleInstance(fn func(fleetName, instanceName string) (*instanceops.Result, error)) func() {
	prev := toggleInstanceStatus
	toggleInstanceStatus = fn
	return func() {
		toggleInstanceStatus = prev
	}
}
