package backend

// ScreenCapture holds the result of capturing a tmux pane's content.
type ScreenCapture struct {
	Content string // visible pane text; empty when capture failed
	OK      bool   // true if the tmux session exists and capture succeeded
}
