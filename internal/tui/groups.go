package tui

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
)

// sessionGroup represents a set of related tmux sessions inside a
// container that should be displayed and managed as a unit. Sessions
// created by splitting the outer tmux all share the same group ID.
//
// Naming convention:
//
//	<instance>~<groupID>       — root session (first pane)
//	<instance>~<groupID>~<hex> — additional panes
//
// The '~' character acts as the group separator.
const groupSep = "~"

// sessionGroup is a collection of sessions sharing a group ID.
type sessionGroup struct {
	GroupID  string        // hex identifier, e.g. "a1b2c3"
	Sessions []tmuxSession // individual tmux sessions in the group
}

// savedGroup stores the state of a group's outer tmux panes so
// they can be restored when switching back.
type savedGroup struct {
	GroupID      string   // group identifier
	InstanceName string   // instance this group belongs to
	Sessions     []string // ordered session names (for pane recreation)
	Layout       string   // tmux layout string (for layout restoration)
	PaneCount    int      // number of shell panes (excluding TUI)
}

// groupSessionName builds a session name for the root session of a new group.
func groupSessionName(sanitizedInstance, groupID string) string {
	return sanitizedInstance + groupSep + groupID
}

// groupMemberName builds a session name for an additional pane in a group.
func groupMemberName(sanitizedInstance, groupID, suffix string) string {
	return sanitizedInstance + groupSep + groupID + groupSep + suffix
}

// parseGroupID extracts the group ID from a session name, if it follows
// the group naming convention. Returns ("", false) for non-grouped sessions.
func parseGroupID(sanitizedInstance, sessionName string) (groupID string, ok bool) {
	prefix := sanitizedInstance + groupSep
	if !strings.HasPrefix(sessionName, prefix) {
		return "", false
	}
	rest := sessionName[len(prefix):]
	// rest is either "groupID" or "groupID~suffix"
	parts := strings.SplitN(rest, groupSep, 2)
	if parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

// groupSessions takes a list of discovered tmux sessions and groups them
// by group ID. Sessions that don't follow the naming convention are
// returned as individual groups (one session per group, using the session
// name as the group ID). Groups are sorted by group ID for stable display.
func groupSessions(sanitizedInstance string, sessions []tmuxSession) []sessionGroup {
	grouped := make(map[string]*sessionGroup)
	var ungrouped []tmuxSession

	for _, s := range sessions {
		gid, ok := parseGroupID(sanitizedInstance, s.Name)
		if !ok {
			ungrouped = append(ungrouped, s)
			continue
		}
		g, exists := grouped[gid]
		if !exists {
			g = &sessionGroup{GroupID: gid}
			grouped[gid] = g
		}
		g.Sessions = append(g.Sessions, s)
	}

	// Build result: ungrouped sessions first (as single-session groups),
	// then grouped sessions sorted by ID.
	var result []sessionGroup
	for _, s := range ungrouped {
		result = append(result, sessionGroup{
			GroupID:  s.Name, // use session name as pseudo-group ID
			Sessions: []tmuxSession{s},
		})
	}

	gids := make([]string, 0, len(grouped))
	for gid := range grouped {
		gids = append(gids, gid)
	}
	sort.Strings(gids)
	for _, gid := range gids {
		result = append(result, *grouped[gid])
	}

	return result
}

// isGroupedSession returns true if the session name follows the group
// naming convention for the given instance.
func isGroupedSession(sanitizedInstance, sessionName string) bool {
	_, ok := parseGroupID(sanitizedInstance, sessionName)
	return ok
}

// randomHex returns a random hex string of n bytes (2n characters).
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
