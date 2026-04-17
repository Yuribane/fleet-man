package tui

import (
	"os"
	"reflect"
	"runtime"
	"testing"
)

func TestClipboardCmdsWayland(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always returns pbcopy")
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	got := clipboardCmds()
	want := []string{"wl-copy", "wl-copy --primary"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clipboardCmds() with WAYLAND_DISPLAY = %q, want %q", got, want)
	}
}

func TestClipboardCmdsX11Fallback(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always returns pbcopy")
	}
	os.Unsetenv("WAYLAND_DISPLAY")
	got := clipboardCmds()
	want := []string{"xclip -sel clip -i", "xclip -sel primary -i"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clipboardCmds() without WAYLAND_DISPLAY = %q, want %q", got, want)
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
