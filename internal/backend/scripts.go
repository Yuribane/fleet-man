package backend

import "strings"

// ===========================================
// Tool Probe
// ===========================================

// ToolProbeScript detects which agent tool (if any) is running inside
// a container. It runs a single `ps` scan, excludes the probe's own
// process group to avoid self-matching, and checks every command-line
// field against all tool names at once. Priority order: claude,
// copilot, codex, gemini.
//
// The matching pattern (^|/)t($|[-./]) recognises:
//   - standalone binaries: `claude`, `/usr/local/bin/claude`
//   - interpreter-wrapped CLIs: `node /opt/copilot-cli/cli.js`
//     (`copilot` appears as a path segment)
//   - versioned paths: `gemini-cli`, `copilot.js`
//
// It rejects substrings like `my-claude-notes.txt` because the tool
// name must start a path component, which avoids false positives from
// files being edited or viewed.
const ToolProbeScript = `MY_PGID=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' \t\n\r')
ps -eo pid,pgid,args --no-headers 2>/dev/null | awk -v pgid="$MY_PGID" '
BEGIN {
  n = 4
  tools[1] = "claude"
  tools[2] = "copilot"
  tools[3] = "codex"
  tools[4] = "gemini"
}
$2 == pgid { next }
{
  for (i = 3; i <= NF; i++) {
    for (j = 1; j <= n; j++) {
      if ($i ~ "(^|/)"tools[j]"($|[-./])") {
        found[j] = 1
      }
    }
  }
}
END {
  for (j = 1; j <= n; j++) {
    if (found[j]) { print tools[j]; exit 0 }
  }
  print "-"
}
'
`

// ParseToolProbeOutput parses the stdout of ToolProbeScript. The
// second return is false only when the probe exec failed (empty
// output). Empty tool with ok=true means the probe ran but found no
// agent.
func ParseToolProbeOutput(output string) (string, bool) {
	tool := strings.TrimSpace(output)
	if tool == "" {
		return "", false
	}
	if tool == "-" {
		return "", true
	}
	return tool, true
}

// ===========================================
// Session Capture
// ===========================================

// sessionMarker is emitted before each session's capture inside
// CaptureAllScript. ASCII RS (\x1e) does not appear in tmux pane
// output, so splitting on this marker is unambiguous.
const sessionMarker = "\x1eFLEET_SESSION_8f4a2b1c\x1e"

// CaptureAllScript lists every tmux session and captures each pane's
// visible content in a single shell invocation. The session name
// precedes each capture on its own marker line so the Go side can
// demultiplex the output back into per-session captures.
const CaptureAllScript = `sessions=$(tmux list-sessions -F "#{session_name}" 2>/dev/null)
[ -z "$sessions" ] && exit 0
printf '%s\n' "$sessions" | while IFS= read -r sess; do
  [ -z "$sess" ] && continue
  printf '\036FLEET_SESSION_8f4a2b1c\036%s\n' "$sess"
  tmux capture-pane -t "$sess" -p 2>/dev/null
done
`

// ParseAllSessionsOutput parses CaptureAllScript output into
// per-session captures. Returns an empty map when the output has no
// session markers (tmux server not running or no sessions).
func ParseAllSessionsOutput(output string) map[string]ScreenCapture {
	result := make(map[string]ScreenCapture)
	if output == "" {
		return result
	}

	var currentName string
	var currentBuf strings.Builder
	flush := func() {
		if currentName == "" {
			return
		}
		content := strings.TrimRight(currentBuf.String(), "\n")
		result[currentName] = ScreenCapture{Content: content, OK: true}
	}

	for _, line := range strings.SplitAfter(output, "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasPrefix(trimmed, sessionMarker) {
			flush()
			currentName = strings.TrimPrefix(trimmed, sessionMarker)
			currentBuf.Reset()
			continue
		}
		if currentName == "" {
			continue
		}
		currentBuf.WriteString(line)
	}
	flush()
	return result
}
