package fleet

import (
	"encoding/json"
	"testing"
)

func TestInstanceGetDisplayNameFallsBackToName(t *testing.T) {
	inst := &Instance{Name: "agent-1"}
	if got := inst.GetDisplayName(); got != "agent-1" {
		t.Fatalf("GetDisplayName() = %q, want %q (fallback to Name)", got, "agent-1")
	}
}

func TestInstanceGetDisplayNameReturnsDisplayNameWhenSet(t *testing.T) {
	inst := &Instance{Name: "agent-1", DisplayName: "auth-fix"}
	if got := inst.GetDisplayName(); got != "auth-fix" {
		t.Fatalf("GetDisplayName() = %q, want %q", got, "auth-fix")
	}
}

// TestInstanceUnmarshalLegacyHasNoDisplayName verifies that Instance records
// persisted before DisplayName existed deserialize cleanly with an empty
// DisplayName, so GetDisplayName falls back to Name.
func TestInstanceUnmarshalLegacyHasNoDisplayName(t *testing.T) {
	legacy := []byte(`{"name":"agent-1","container_id":"abc","config":"x","workspace_dir":"/tmp","status":"running"}`)
	var inst Instance
	if err := json.Unmarshal(legacy, &inst); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if inst.DisplayName != "" {
		t.Fatalf("DisplayName = %q, want empty for legacy state", inst.DisplayName)
	}
	if got := inst.GetDisplayName(); got != "agent-1" {
		t.Fatalf("GetDisplayName() = %q, want %q", got, "agent-1")
	}
}
