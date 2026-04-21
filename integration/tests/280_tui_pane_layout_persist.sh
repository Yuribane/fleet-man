#!/usr/bin/env bash
# Description: Tmux pane layout persists across fleet restarts (issue #16)
set -euo pipefail

source "$(dirname "$0")/../common.sh"
itest_cleanup() { tui_kill; }
itest_begin

setup_test
fleet_up alpha

# ---------------------------------------------------------------------------
# Phase 1 — build a multi-pane group with a non-default orientation.
#
# The bug in #16 is that the user's split orientation and pane sizes get
# lost on restart — a group built with side-by-side panes comes back
# stacked. We deliberately pick a layout that does NOT match the default
# placeholder geometry restoreGroupCmd() would synthesise without the
# saved layout, so the assertion at the bottom distinguishes "any layout
# restored" from "saved layout restored".
#
#   default placeholder : TUI | {shell1 / shell2}   (stacked)
#   this test's layout  : TUI | {shell1 | shell2}   (side-by-side)
# ---------------------------------------------------------------------------
info "Launch TUI and expand alpha"
tui_spawn
tui_wait_for "alpha" 15
tui_wait_for "○ idle" 60

tui_send j
sleep 0.5
tui_send Space
tui_wait_for "+ new session" 15

info "Open alpha shell in a split pane (first group session)"
tui_send Enter
tui_wait_for_pane 2 20
sleep 3

# Discover the group ID via the outer pane title that `fleet shell` sets
# with select-pane -T. Title format: "<sanitized-instance>~<groupID>".
pane_title=$(tmux display-message -t "${TUI_SESSION}:.1" -p "#{pane_title}" 2>/dev/null || true)
info "pane 1 title: ${pane_title}"
group_id="${pane_title#*~}"
if [ -z "${group_id}" ] || [ "${group_id}" = "${pane_title}" ]; then
  fail "could not parse group id from pane title: ${pane_title}"
fi
info "group id: ${group_id}"

info "Add a second pane side-by-side with the first via fleet shell --group"
# The %/" bindings are what a user would press, but tmux send-keys
# bypasses bindings — invoke tmux split-window directly with the same
# command the binding installs. -h gives the side-by-side layout that
# the default restore would otherwise get wrong.
tmux split-window -h -t "${TUI_SESSION}:.1" \
  "env TERM=xterm-256color ${FLEET_BIN} shell ${FIXTURE_REPO_NAME}/alpha --group ${group_id}"
tui_wait_for_pane 3 20
sleep 5  # let the second fleet shell attach + set its outer pane title

layout_before=$(tmux display-message -t "${TUI_SESSION}" -p "#{window_layout}")
info "layout before quit: ${layout_before}"

# ---------------------------------------------------------------------------
# Phase 2 — clean quit so View() runs saveCurrentGroupLayout().
# ---------------------------------------------------------------------------
info "Focus TUI pane and press 'q' for a clean quit"
tmux select-pane -t "${TUI_SESSION}:.0"
tui_send_pane 0 q

info "Wait for tmux session to exit (fleet quit tears down pane 0)"
deadline=$(( $(date +%s) + 15 ))
while [ "$(date +%s)" -lt "${deadline}" ]; do
  if ! tmux has-session -t "${TUI_SESSION}" 2>/dev/null; then
    break
  fi
  sleep 0.25
done
if tmux has-session -t "${TUI_SESSION}" 2>/dev/null; then
  fail "tmux session did not exit after q — fleet did not quit cleanly"
fi

# ---------------------------------------------------------------------------
# Phase 3 — verify state.json persisted the layout.
# ---------------------------------------------------------------------------
info "Verify state.json contains the saved group layout"
assert_file_exists "${HOME}/.fleet/state.json"
state=$(cat "${HOME}/.fleet/state.json")
assert_contains "${state}" '"groupLayouts"' "state.json missing groupLayouts field"
assert_contains "${state}" "\"${group_id}\"" "state.json missing entry for group ${group_id}"
assert_contains "${state}" '"paneCount": 2' "state.json should record 2 shell panes"

saved_layout=$(grep -oE '"layout"[[:space:]]*:[[:space:]]*"[^"]*"' "${HOME}/.fleet/state.json" \
  | head -1 \
  | sed -E 's/.*:[[:space:]]*"([^"]*)".*/\1/')
info "saved layout: ${saved_layout}"
[ -n "${saved_layout}" ] || fail "could not extract saved layout from state.json"

# ---------------------------------------------------------------------------
# Phase 4 — respawn fleet and restore the group.
# ---------------------------------------------------------------------------
info "Respawn fleet"
tui_spawn
tui_wait_for "alpha" 15
tui_wait_for "○ idle" 60

info "Expand alpha; the saved group should appear with (2 panes) label"
tui_send j
sleep 0.5
tui_send Space
tui_wait_for "(2 panes)" 20

info "Move to the group row and open it"
tui_send j
sleep 0.3
tui_send Enter
tui_wait_for_pane 3 30
sleep 3

# ---------------------------------------------------------------------------
# Phase 5 — assert restored layout structure matches the saved one.
#
# window_layout strings include per-pane tmux IDs that change across
# runs, so string equality is too strict. We compare the bracket
# structure instead: tmux uses '{' for side-by-side and '[' for stacked
# splits, so the sequence of braces encodes orientation and nesting
# unambiguously. For this test:
#   with fix    : {{}}  (outer TUI|group, inner shell|shell)
#   without fix : {[]}  (outer TUI|group, inner shell/shell stacked)
# ---------------------------------------------------------------------------
layout_after=$(tmux display-message -t "${TUI_SESSION}" -p "#{window_layout}")
info "layout after restore: ${layout_after}"

brace_before=$(printf '%s' "${layout_before}" | tr -cd '{}[]')
brace_after=$(printf '%s' "${layout_after}"  | tr -cd '{}[]')
info "brace structure before=[${brace_before}] after=[${brace_after}]"
assert_equals "${brace_before}" "${brace_after}" \
  "restored layout must preserve split orientation from saved layout"

pass "Tmux pane layout persists across fleet restarts"
