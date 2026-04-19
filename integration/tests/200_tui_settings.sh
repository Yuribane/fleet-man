#!/usr/bin/env bash
# Description: TUI settings page opens from fleet list and closes with esc and q
set -euo pipefail

source "$(dirname "$0")/../common.sh"
itest_cleanup() { tui_kill; }
itest_begin

setup_test
fleet_up alpha

tui_spawn
tui_wait_for "alpha" 15

tui_assert_contains "settings" "settings row missing"

# Move cursor up from header, wrapping to the last row (settings), then Enter.
tui_send k
sleep 0.3
tui_send Enter

# Settings page should render.
tui_wait_for "Tmux vim keys" 5
tui_assert_contains "General" "General section missing"
tui_assert_contains "Show help text" "Show help text row missing"

# Close with Escape; fleet list should come back.
tui_send Escape
tui_wait_for "alpha" 5
tui_wait_for_absent "Tmux vim keys" 5

# Re-open settings and close with q this time.
tui_send k
sleep 0.3
tui_send Enter
tui_wait_for "Tmux vim keys" 5

tui_send q
tui_wait_for "alpha" 5
tui_wait_for_absent "Tmux vim keys" 5

pass "TUI settings page open + close (esc and q)"
