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

func TestParseBackendType(t *testing.T) {
	tests := []struct {
		value   string
		want    BackendType
		wantErr bool
	}{
		{value: "devcontainer", want: BackendDevcontainer},
		{value: "coder", want: BackendCoder},
		{value: "codespaces", want: BackendCodespaces},
		{value: "bogus", wantErr: true},
		{value: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseBackendType(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseBackendType() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBackendType() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseBackendType() = %q, want %q", got, tt.want)
			}
		})
	}
}
