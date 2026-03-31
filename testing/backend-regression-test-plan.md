# Backend Strategy Pattern — Regression Test Plan

Covers all interaction points migrated from `internal/devcontainer` to
`internal/backend` + `internal/backend/devcontainer`.

---

## 1. Unit Tests (automated)

Run with `go test ./...`. All tests below exist and pass.

| Package | Test | What it covers |
|---------|------|----------------|
| `backend/devcontainer` | `TestParseToolProbeOutput` | Agent tool detection parsing (claude, copilot, codex, gemini, no-agent, empty) |
| `backend/devcontainer` | `TestScreenCaptureZeroValue` | Zero-value ScreenCapture has OK=false |
| `backend/devcontainer` | `TestHostSSHAuthSock_*` (4 tests) | SSH socket detection: unset, nonexistent, regular file, valid socket |
| `backend/devcontainer` | `TestSSHUpArgs_*` (2 tests) | SSH bind-mount args with/without socket |
| `backend/devcontainer` | `TestSSHExecArgs_*` (2 tests) | SSH exec env args with/without env var |
| `backend/devcontainer` | `TestExecArgs_*` (2 tests) | Full devcontainer exec arg construction |
| `instanceops` | `TestStopInstance*` (2 tests) | Stop transition + no-op when already stopped |
| `instanceops` | `TestStartInstance*` (2 tests) | Start transition + no-op when already running |
| `tui` | `TestActivityTrackerUpdate` (9 sub-tests) | Screen diff → working/waiting/idle, tool detection, capture failure preservation, container cleanup |
| `tui` | `TestUpdateNormal*` (6 tests) | Stop/start shortcut, cursor wrapping, creating/failed guards |
| `tui` | `TestViewFleetList*` (6 tests) | Agent indicators (working/waiting/idle/off), branch display |
| `tui` | `TestUpdateSettings*` (6 tests) | Tool cycling, dotfiles editing, esc/enter, navigation |
| `cli` | `TestListCmd*` | CLI list command output |

---

## 2. TUI Integration Tests (manual, via tmux)

Launch: `tmux new-session -d -s test -x 140 -y 35 'fleet'`
Capture: `tmux capture-pane -t test -p`

### 2.1 Startup & Rendering

| # | Action | Expected |
|---|--------|----------|
| 1 | Launch TUI | Fleet logo renders, fleet list with instances visible |
| 2 | Wait 3s | Stats (CPU mcpu, MB) and agent status appear for running instances |
| 3 | Verify branch | Branch name (e.g., `main`) shown next to running instances |

### 2.2 Navigation

| # | Action | Expected |
|---|--------|----------|
| 4 | Press `j` | Cursor `>` moves down |
| 5 | Press `k` | Cursor `>` moves up |
| 6 | Press `j` past last row | Cursor wraps to first row |
| 7 | Press `k` past first row | Cursor wraps to last row |

### 2.3 Collapse/Expand

| # | Action | Expected |
|---|--------|----------|
| 8 | Select fleet header, press `Space` | Arrow changes `▼` → `▶`, instances hidden |
| 9 | Press `Space` again | Arrow changes `▶` → `▼`, instances visible |

### 2.4 Stop/Start (`Backend.Stop`, `Backend.Start`)

| # | Action | Expected |
|---|--------|----------|
| 10 | Select running instance, press `s` | Status changes to `stopped`, message "Stopped fleet/instance" |
| 11 | Press `s` again | Status changes to `running`, message "Started fleet/instance" |
| 12 | Select creating instance, press `s` | Message "Instance ... is still creating" (no action) |
| 13 | Select failed instance, press `s` | Message "Instance ... is failed and cannot be toggled" |

### 2.5 Exec (`Backend.ExecCommand`, `execWithBannerCmd`)

| # | Action | Expected |
|---|--------|----------|
| 14 | Select running instance, press `Enter` | Screen clears, banner renders, drops into tmux session inside container |
| 15 | Inside session, press `Ctrl+Q` | Detaches back to TUI cleanly |
| 16 | Press `Enter` again | Reattaches to same tmux session (persistent) |

### 2.6 Open in Terminal (`Backend.ExecCommand`)

| # | Action | Expected |
|---|--------|----------|
| 17 | Select running instance, press `o` | New terminal window opens (if in tmux: new tmux window) |

### 2.7 Logs (`Backend.LogsCommand`)

| # | Action | Expected |
|---|--------|----------|
| 18 | Select running instance, press `l` | Container logs displayed, then returns to TUI |

### 2.8 VS Code (`Backend.EditorURI`)

| # | Action | Expected |
|---|--------|----------|
| 19 | Select running instance, press `c` | VS Code opens with devcontainer remote URI (requires VS Code installed) |

### 2.9 Delete Dialogs (`Backend.Down` via `m.dc`)

| # | Action | Expected |
|---|--------|----------|
| 20 | Select instance, press `d` | Delete confirmation dialog appears |
| 21 | Press `n` | Dialog dismissed, message "Cancelled" |
| 22 | Select fleet header, press `d` | Fleet-level delete dialog appears |
| 23 | Press `y` | Double-confirm WARNING dialog appears (if fleet has instances) |
| 24 | Press `n` | Dialog dismissed, fleet preserved |

### 2.10 Add Instance / New Fleet Dialogs

| # | Action | Expected |
|---|--------|----------|
| 25 | Select fleet header, press `a` | Add instance dialog with fleet name and name input |
| 26 | Press `Esc` | Dialog dismissed |
| 27 | Press `n` | New fleet dialog with repo URL input |
| 28 | Press `Esc` | Dialog dismissed |

### 2.11 Settings Page (Agent Tool Selection)

| # | Action | Expected |
|---|--------|----------|
| 29 | Navigate to settings, press `Enter` | Settings page renders with tool selection and dotfiles config |
| 30 | Press `Right` repeatedly | Tool cycles: Claude Code → Gemini → Copilot → Codex → Claude Code |
| 31 | Press `Left` | Tool cycles in reverse |
| 32 | Press `Esc` | Returns to fleet list |

### 2.12 Stats Polling (`Backend.Stats`, `CaptureScreens`, `AgentToolProbes`)

| # | Action | Expected |
|---|--------|----------|
| 33 | Wait with running instances | CPU (mcpu) and Memory (MB) update every ~3s |
| 34 | Agent with tool running, screen changing | Shows `▶ <tool>` (working) |
| 35 | Agent with tool running, screen static | Shows `⏸ <tool>` (waiting) |
| 36 | No agent running | Shows `○ idle` |
| 37 | Stopped instance | No agent indicator or stats shown |

### 2.13 Real Agent Detection (Claude, Copilot, Codex)

Tests require real agent CLIs installed inside containers (`npm install -g @anthropic-ai/claude-code @openai/codex @github/copilot`). Each agent must be launched inside a tmux session (created by exec-ing into the instance via TUI).

| # | Action | Expected |
|---|--------|----------|
| 38 | Run `claude` interactively in claude-agent container | Probe detects `claude`, TUI shows `⏸ Claude Code` or `▶ Claude Code` |
| 39 | Run `copilot` in copilot-agent container | Probe detects `copilot`, TUI shows `⏸ Copilot` |
| 40 | Run `codex` in codex-agent container | Probe detects `codex`, TUI shows `⏸ Codex` |
| 41 | Claude UI actively rendering (screen changing ≥3 chars within 12s) | State transitions to `▶ Claude Code` (working) |
| 42 | Claude UI static (waiting for input, no screen changes) | State transitions to `⏸ Claude Code` (waiting) |
| 43 | Kill claude process (`pkill -f claude`) | State transitions to `○ idle` |
| 44 | Kill copilot process | State transitions to `○ idle` |
| 45 | Kill codex process | State transitions to `○ idle` |
| 46 | All three agents running simultaneously | All three detected independently with correct labels |

### 2.14 Refresh

| # | Action | Expected |
|---|--------|----------|
| 47 | Press `r` | State reloaded, message "Refreshed" |

---

## 3. CLI Integration Tests (manual)

| # | Command | Expected |
|---|---------|----------|
| 48 | `fleet list` | Table header rendered |
| 49 | `fleet list <fleet>` | Instances listed with status, container ID, branch |
| 50 | `fleet status` | Fleet summary with running/stopped counts |
| 51 | `fleet stop <fleet/instance>` | Prints "Instance ... stopped." |
| 52 | `fleet start <fleet/instance>` | Prints "Instance ... started." |
| 53 | `fleet exec <fleet/instance> ls` | Runs `ls` inside container, outputs result |
| 54 | `fleet logs <fleet/instance>` | Streams container logs |
| 55 | `fleet logs -f <fleet/instance>` | Streams logs with follow (Ctrl+C to exit) |
| 56 | `fleet code <fleet/instance>` | Opens VS Code (requires `code` CLI) |
| 57 | `fleet down <fleet/instance>` | Removes instance, prints confirmation |
| 58 | `fleet destroy <fleet>` | Removes all instances in fleet |
| 59 | `fleet up <name>` | Creates instance (git clone + devcontainer up) |

---

## 4. Build Verification

| # | Check | Command |
|---|-------|---------|
| 60 | Clean build | `go build ./...` |
| 61 | All unit tests pass | `go test ./...` |
| 62 | No vet warnings | `go vet ./...` |
| 63 | No stale imports | `grep -r 'internal/devcontainer"' internal/` returns nothing |

---

## 5. Test Results Summary (2026-03-30)

### Automated Tests
- `go build ./...` — PASS
- `go test ./...` — PASS (all 8 test packages)
- `go vet ./...` — PASS
- No stale `internal/devcontainer` imports — PASS

### TUI Tests (via tmux)
- Startup & rendering — PASS
- Navigation (j/k, wrap) — PASS
- Collapse/expand (Space) — PASS
- Stop/Start (s key → `Backend.Stop`/`Start`) — PASS
- Exec (Enter → `Backend.ExecCommand` + `execWithBannerCmd`) — PASS
- Detach (Ctrl+Q) — PASS
- Logs (l key → `Backend.LogsCommand`) — PASS
- Delete dialog (d key → `Backend.Down` via `m.dc`) — PASS
- Delete cancel — PASS
- Fleet-level delete with double-confirm — PASS
- Add instance dialog (a key) — PASS
- New fleet dialog (n key) — PASS
- Settings: tool cycling Claude Code/Gemini/Copilot/Codex — PASS
- Settings: esc to return — PASS
- Refresh (r key) — PASS
- Stats polling (CPU, memory, agent status) — PASS

### Real Agent Detection Tests (via tmux, real CLIs)
- Claude Code detection (`claude` process → `⏸ Claude Code`) — PASS
- Copilot detection (`copilot` process → `⏸ Copilot`) — PASS
- Codex detection (`codex` process → `⏸ Codex`) — PASS
- All three agents detected simultaneously — PASS
- Working state (`▶ Claude Code` when screen actively changing) — PASS
- Waiting state (`⏸ Claude Code` when screen static) — PASS
- Working → Waiting transition (screen stops changing) — PASS
- Agent exit → Idle transition (`○ idle` after `pkill`) — PASS

### CLI Tests
- `fleet list` / `fleet list <fleet>` — PASS
- `fleet status` — PASS
- `fleet stop` / `fleet start` — PASS
- `fleet exec <fleet/instance> <cmd>` — PASS
- `fleet logs` — PASS
