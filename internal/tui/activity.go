package tui

import (
	"time"

	"github.com/BenjaminBenetti/fleet-man/internal/backend"
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
// diffing consecutive tmux screen captures. State is aggregated
// across every session inside a container — a working agent in any
// session marks the container as working.
type ActivityTracker struct {
	states     map[string]agentState
	tools      map[string]state.AgentTool
	prevScreen map[string]map[string]string    // containerID → sessionName → last content
	lastChange map[string]map[string]time.Time // containerID → sessionName → last change
}

// NewActivityTracker returns an initialized tracker.
func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{
		states:     make(map[string]agentState),
		tools:      make(map[string]state.AgentTool),
		prevScreen: make(map[string]map[string]string),
		lastChange: make(map[string]map[string]time.Time),
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

// Update processes new captures and probe results to derive agent
// states and tool identifications.
//
// Tool detection uses process-based probes (ps aux inside containers)
// and is independent of screen capture success — a probe finding
// claude is recorded even if every tmux capture failed transiently.
//
// State detection iterates over every session captured per container:
//   - Container missing from captures → preserve previous state
//   - Container OK=false → agentNotRunning (exec failed entirely)
//   - Container OK=true with no sessions → agentNotRunning
//   - Container OK=true with sessions → diff each session against its
//     previous capture; container is agentWorking if ANY session had
//     ≥ threshold change within the activity window
//
// Containers absent from expectedIDs are dropped (cleanup).
func (t *ActivityTracker) Update(captures map[string]backend.AllSessions, probes map[string]string, expectedIDs []string, now time.Time) {
	newStates := make(map[string]agentState, len(expectedIDs))
	newTools := make(map[string]state.AgentTool, len(expectedIDs))
	newPrev := make(map[string]map[string]string, len(expectedIDs))
	newLastChange := make(map[string]map[string]time.Time, len(expectedIDs))

	for _, id := range expectedIDs {
		all, captured := captures[id]
		if !captured || !all.OK {
			// Capture exec failed — preserve previous state to avoid flicker
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

		// Tool detection runs independently of session capture — a
		// probe that found claude must be recorded even when no
		// sessions match a tracked baseline.
		probeConfirmedEmpty := false
		if probes != nil {
			if probeTool, probed := probes[id]; probed {
				if probeTool != "" {
					newTools[id] = state.AgentTool(probeTool)
				} else {
					probeConfirmedEmpty = true
				}
			} else if tool, ok := t.tools[id]; ok {
				newTools[id] = tool
			}
		} else if tool, ok := t.tools[id]; ok {
			newTools[id] = tool
		}

		// Snapshot every captured session as the new baseline. Doing
		// this even when probeConfirmedEmpty / no-sessions ensures
		// the next cycle has prev content to diff against once an
		// agent starts.
		nextPrev := make(map[string]string, len(all.Sessions))
		for sName, sc := range all.Sessions {
			if sc.OK {
				nextPrev[sName] = sc.Content
			}
		}
		newPrev[id] = nextPrev

		if probeConfirmedEmpty {
			newStates[id] = agentNotRunning
			continue
		}

		if len(all.Sessions) == 0 {
			newStates[id] = agentNotRunning
			continue
		}

		prevBySession := t.prevScreen[id]
		lcBySession := t.lastChange[id]

		nextLC := make(map[string]time.Time, len(all.Sessions))
		anyHadHistory := false
		workingDetected := false

		for sName, sc := range all.Sessions {
			if !sc.OK {
				continue
			}
			prev, hasPrev := prevBySession[sName]
			var lc time.Time
			if lcBySession != nil {
				lc = lcBySession[sName]
			}
			if hasPrev {
				anyHadHistory = true
				if countDiffs(prev, sc.Content) >= screenChangeThreshold {
					lc = now
				}
			}
			nextLC[sName] = lc
			if !lc.IsZero() && now.Sub(lc) < screenActivityWindow {
				workingDetected = true
			}
		}
		newLastChange[id] = nextLC

		switch {
		case workingDetected:
			newStates[id] = agentWorking
		case anyHadHistory:
			newStates[id] = agentWaiting
		default:
			// First capture(s) — no history yet, assume waiting
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
