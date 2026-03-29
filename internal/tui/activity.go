package tui

import (
	"strings"
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/devcontainer"
	"github.com/BenjaminBenetti/fleet-man/internal/state"
)

type agentState int

const (
	agentNotRunning agentState = iota
	agentWorking
	agentWaiting
)

// screenChangeThreshold is the minimum number of characters that must
// differ between consecutive screen captures to count as meaningful
// activity (catches spinner animations while ignoring cursor blink).
const screenChangeThreshold = 3

// screenActivityWindow is how recently a meaningful screen change must
// have occurred for the agent to be considered actively working.
const screenActivityWindow = 12 * time.Second

// ActivityTracker determines agent working/waiting/idle state by
// diffing consecutive tmux screen captures.
type ActivityTracker struct {
	states     map[string]agentState
	tools      map[string]state.AgentTool
	prevScreen map[string]string
	lastChange map[string]time.Time
}

// NewActivityTracker returns an initialized tracker.
func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{
		states:     make(map[string]agentState),
		tools:      make(map[string]state.AgentTool),
		prevScreen: make(map[string]string),
		lastChange: make(map[string]time.Time),
	}
}

// State returns the derived agent state for a container.
func (t *ActivityTracker) State(containerID string) agentState {
	return t.states[containerID]
}

// Tool returns the detected tool for a container.
func (t *ActivityTracker) Tool(containerID string) state.AgentTool {
	return t.tools[containerID]
}

// Update processes new screen captures and derives agent states.
//
// For each container in expectedIDs:
//   - Missing from captures → preserve previous state (transient failure)
//   - Present with OK=false → agentNotRunning (no tmux session)
//   - Present with OK=true  → diff against previous capture to determine
//     working (≥3 chars changed within 12s) or waiting
//
// Containers not in expectedIDs are cleaned up.
func (t *ActivityTracker) Update(captures map[string]devcontainer.ScreenCapture, expectedIDs []string, now time.Time) {
	newStates := make(map[string]agentState, len(expectedIDs))
	newTools := make(map[string]state.AgentTool, len(expectedIDs))
	newPrev := make(map[string]string, len(expectedIDs))
	newLastChange := make(map[string]time.Time, len(expectedIDs))

	for _, id := range expectedIDs {
		sc, captured := captures[id]
		if !captured {
			// Capture failed — preserve previous state to avoid flicker
			if prev, ok := t.states[id]; ok {
				newStates[id] = prev
			}
			if tool, ok := t.tools[id]; ok {
				newTools[id] = tool
			}
			if s, ok := t.prevScreen[id]; ok {
				newPrev[id] = s
			}
			if lc, ok := t.lastChange[id]; ok {
				newLastChange[id] = lc
			}
			continue
		}

		if !sc.OK {
			newStates[id] = agentNotRunning
			continue
		}

		// Detect which tool is running from screen content
		if tool := detectTool(sc.Content); tool != "" {
			newTools[id] = tool
		} else if tool, ok := t.tools[id]; ok {
			newTools[id] = tool // preserve previous detection
		}

		// Compare with previous screen
		prev, hasPrev := t.prevScreen[id]
		lc := t.lastChange[id]

		if hasPrev && countDiffs(prev, sc.Content) >= screenChangeThreshold {
			lc = now
		}

		newPrev[id] = sc.Content
		newLastChange[id] = lc

		if !lc.IsZero() && now.Sub(lc) < screenActivityWindow {
			newStates[id] = agentWorking
		} else if hasPrev {
			newStates[id] = agentWaiting
		} else {
			// First capture — no history yet, assume waiting
			newStates[id] = agentWaiting
		}
	}

	t.states = newStates
	t.tools = newTools
	t.prevScreen = newPrev
	t.lastChange = newLastChange
}

// countDiffs returns the number of character positions that differ
// between two strings, plus any length difference.
func countDiffs(a, b string) int {
	diffs := 0
	ar, br := []rune(a), []rune(b)
	minLen := len(ar)
	if len(br) < minLen {
		minLen = len(br)
	}
	for i := 0; i < minLen; i++ {
		if ar[i] != br[i] {
			diffs++
		}
	}
	if len(ar) > minLen {
		diffs += len(ar) - minLen
	} else {
		diffs += len(br) - minLen
	}
	return diffs
}

// detectTool identifies the agent tool from screen content by looking
// for each tool's name in the captured terminal output.
// Order matters: copilot is checked before claude because Copilot's
// UI displays its backend model name (e.g. "Claude Sonnet 4.5").
func detectTool(screen string) state.AgentTool {
	lower := strings.ToLower(screen)
	switch {
	case strings.Contains(lower, "copilot"):
		return state.AgentToolCopilot
	case strings.Contains(lower, "claude"):
		return state.AgentToolClaude
	case strings.Contains(lower, "codex"):
		return state.AgentToolCodex
	case strings.Contains(lower, "gemini"):
		return state.AgentToolGemini
	default:
		return ""
	}
}
