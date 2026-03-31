package devcontainer

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestHostSSHAuthSock_Unset(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if got := hostSSHAuthSock(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHostSSHAuthSock_NonexistentPath(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/no-such-socket-"+t.Name())
	if got := hostSSHAuthSock(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHostSSHAuthSock_RegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-socket")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Setenv("SSH_AUTH_SOCK", f.Name())
	if got := hostSSHAuthSock(); got != "" {
		t.Errorf("expected empty for regular file, got %q", got)
	}
}

func TestHostSSHAuthSock_ValidSocket(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	t.Setenv("SSH_AUTH_SOCK", sockPath)
	if got := hostSSHAuthSock(); got != sockPath {
		t.Errorf("expected %q, got %q", sockPath, got)
	}
}

func TestSSHUpArgs_WithValidSocket(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	t.Setenv("SSH_AUTH_SOCK", sockPath)
	args := sshUpArgs()
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	if args[0] != "--mount" {
		t.Errorf("args[0] = %q, want --mount", args[0])
	}
	wantMount := "type=bind,source=" + sockPath + ",target=" + containerSSHSocketPath
	if args[1] != wantMount {
		t.Errorf("args[1] = %q, want %q", args[1], wantMount)
	}
	if args[2] != "--remote-env" {
		t.Errorf("args[2] = %q, want --remote-env", args[2])
	}
	wantEnv := "SSH_AUTH_SOCK=" + containerSSHSocketPath
	if args[3] != wantEnv {
		t.Errorf("args[3] = %q, want %q", args[3], wantEnv)
	}
}

func TestSSHUpArgs_NoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if args := sshUpArgs(); args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestSSHExecArgs_WithEnvVar(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/some/path")
	args := sshExecArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "--remote-env" {
		t.Errorf("args[0] = %q, want --remote-env", args[0])
	}
	wantEnv := "SSH_AUTH_SOCK=" + containerSSHSocketPath
	if args[1] != wantEnv {
		t.Errorf("args[1] = %q, want %q", args[1], wantEnv)
	}
}

func TestSSHExecArgs_NoEnvVar(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if args := sshExecArgs(); args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestExecArgs_WithSSH(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/some/path")
	args := execArgs("/workspace", []string{"bash"})
	expected := []string{
		"exec", "--workspace-folder", "/workspace",
		"--remote-env", "SSH_AUTH_SOCK=" + containerSSHSocketPath,
		"bash",
	}
	if len(args) != len(expected) {
		t.Fatalf("got %v, want %v", args, expected)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], expected[i])
		}
	}
}

func TestExecArgs_WithoutSSH(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	args := execArgs("/workspace", []string{"bash"})
	expected := []string{"exec", "--workspace-folder", "/workspace", "bash"}
	if len(args) != len(expected) {
		t.Fatalf("got %v, want %v", args, expected)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], expected[i])
		}
	}
}
