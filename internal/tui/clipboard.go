package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/platform"
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
	clipCmds   []string
	lastBuffer string
}

// clipboardCmds returns the system clipboard commands for the current
// platform and display server. On Linux, two commands are returned so
// the content is written to both the regular clipboard (Ctrl+V) and
// the primary selection (middle-click). On macOS, only pbcopy is
// returned because there is no primary selection concept.
func clipboardCmds() []string {
	return clipboardCmdsFor(runtime.GOOS, os.Getenv("WAYLAND_DISPLAY") != "", platform.IsWSL())
}

func clipboardCmdsFor(goos string, hasWayland, isWSL bool) []string {
	if goos == "darwin" {
		return []string{"pbcopy"}
	}
	if goos == "linux" && isWSL {
		return []string{`powershell.exe -NoProfile -Command "Set-Clipboard -Value ([Console]::In.ReadToEnd())"`}
	}
	if hasWayland {
		return []string{"wl-copy", "wl-copy --primary"}
	}
	return []string{"xclip -sel clip -i", "xclip -sel primary -i"}
}

// newClipboardSync creates a clipboard synchroniser. Returns nil if
// tmux is unavailable or no system clipboard tool can be found.
func newClipboardSync() *clipboardSync {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil
	}
	commands := clipboardCmds()
	firstBinary := strings.SplitN(commands[0], " ", 2)[0]
	if _, err := exec.LookPath(firstBinary); err != nil {
		return nil
	}
	return &clipboardSync{clipCmds: commands}
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

	quoted := shQuote(buf)
	for _, clipCmd := range cs.clipCmds {
		cmd := exec.CommandContext(ctx, "sh", "-c",
			fmt.Sprintf("printf '%%s' %s | %s", quoted, clipCmd))
		_ = cmd.Run()
	}
}
