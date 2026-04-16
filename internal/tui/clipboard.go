package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ===========================================
// Clipboard Sync
// ===========================================

// clipboardSync polls the outer tmux paste buffer and copies changes
// to the system clipboard. This provides clipboard support for ALL
// terminal emulators — including those without OSC 52 (e.g. GNOME
// Terminal, Ptyxis). For terminals that DO support OSC 52 (e.g.
// Alacritty, Kitty), the goroutine is harmless since it re-copies
// the same content that OSC 52 already placed on the clipboard.
type clipboardSync struct {
	clipCmd    string
	lastBuffer string
}

// clipboardCmd returns the system clipboard command for the current
// platform and display server.
func clipboardCmd() string {
	if runtime.GOOS == "darwin" {
		return "pbcopy"
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wl-copy"
	}
	return "xclip -sel clip -i"
}

// newClipboardSync creates a clipboard synchroniser. Returns nil if
// tmux is unavailable or no system clipboard tool can be found.
func newClipboardSync() *clipboardSync {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil
	}
	cmd := clipboardCmd()
	bin := strings.SplitN(cmd, " ", 2)[0]
	if _, err := exec.LookPath(bin); err != nil {
		return nil
	}
	return &clipboardSync{clipCmd: cmd}
}

// Start begins polling the tmux paste buffer and synchronising
// changes to the system clipboard. Blocks until ctx is cancelled;
// call from a goroutine.
func (cs *clipboardSync) Start(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cs.poll(ctx)
		}
	}
}

// poll reads the current tmux paste buffer and, if it has changed,
// writes the new content to the system clipboard.
func (cs *clipboardSync) poll(ctx context.Context) {
	out, err := exec.CommandContext(ctx, "tmux", "show-buffer").Output()
	if err != nil {
		// Buffer empty or tmux error — treat as unchanged.
		return
	}
	buf := string(out)
	if buf == cs.lastBuffer || buf == "" {
		return
	}
	cs.lastBuffer = buf

	cmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf("printf '%%s' %s | %s", shQuote(buf), cs.clipCmd))
	_ = cmd.Run()
}
