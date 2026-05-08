package backend

// AllSessions holds tmux screen captures for every session discovered
// inside a single container in one shell invocation.
//
//   - OK=false means the exec itself failed (transient — preserve prev state)
//   - OK=true with empty Sessions means tmux has no sessions (agent not running)
//   - OK=true with populated Sessions means each named session has a capture
type AllSessions struct {
	Sessions map[string]ScreenCapture // sessionName → capture
	OK       bool
}
