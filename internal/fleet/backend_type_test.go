package fleet

import "testing"

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
