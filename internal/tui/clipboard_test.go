package tui

import (
	"reflect"
	"runtime"
	"testing"
)

func TestClipboardCmdsWayland(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always returns pbcopy")
	}
	got := clipboardCmdsFor("linux", true, false)
	want := []string{"wl-copy", "wl-copy --primary"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clipboardCmds() with WAYLAND_DISPLAY = %q, want %q", got, want)
	}
}

func TestClipboardCmdsX11Fallback(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always returns pbcopy")
	}
	got := clipboardCmdsFor("linux", false, false)
	want := []string{"xclip -sel clip -i", "xclip -sel primary -i"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clipboardCmds() without WAYLAND_DISPLAY = %q, want %q", got, want)
	}
}

func TestClipboardCmdsWSL(t *testing.T) {
	got := clipboardCmdsFor("linux", false, true)
	want := []string{`powershell.exe -NoProfile -Command "Set-Clipboard -Value ([Console]::In.ReadToEnd())"`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clipboardCmds() on WSL = %q, want %q", got, want)
	}
}

func TestNewClipboardSyncNoTmux(t *testing.T) {
	// With a non-existent tmux path, newClipboardSync should return nil.
	t.Setenv("PATH", "/nonexistent")
	cs := newClipboardSync()
	if cs != nil {
		t.Error("newClipboardSync() should return nil when tmux is not found")
	}
}
