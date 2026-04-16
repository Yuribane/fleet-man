package tui

import (
	"os"
	"runtime"
	"testing"
)

func TestClipboardCmdWayland(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always returns pbcopy")
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	got := clipboardCmd()
	if got != "wl-copy" {
		t.Errorf("clipboardCmd() with WAYLAND_DISPLAY = %q, want %q", got, "wl-copy")
	}
}

func TestClipboardCmdX11Fallback(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always returns pbcopy")
	}
	os.Unsetenv("WAYLAND_DISPLAY")
	got := clipboardCmd()
	if got != "xclip -sel clip -i" {
		t.Errorf("clipboardCmd() without WAYLAND_DISPLAY = %q, want %q", got, "xclip -sel clip -i")
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
