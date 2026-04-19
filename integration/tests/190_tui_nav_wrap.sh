#!/usr/bin/env bash
# Description: TUI cursor navigation with j/k wraps around top/bottom of list
set -euo pipefail

source "$(dirname "$0")/../common.sh"
itest_cleanup() { tui_kill; }
itest_begin

setup_test
fleet_up alpha

tui_spawn
tui_wait_for "alpha" 15
tui_wait_for "running" 5

# Step 3: cursor starts on the fleet header — verify via Space toggling collapse.
tui_assert_contains "▼ itest-fleet" "fleet should start expanded"
info "collapsing fleet to confirm cursor starts on header"
tui_send Space
tui_wait_for "▶ itest-fleet" 5
info "re-expanding fleet"
tui_send Space
tui_wait_for "▼ itest-fleet" 5

# Step 4: press j three times — should wrap back to the fleet header.
info "pressing j three times to wrap back to header"
tui_send j
sleep 0.2
tui_send j
sleep 0.2
tui_send j
sleep 0.2
tui_send Space
tui_wait_for "▶ itest-fleet" 5
info "j wrapped back to header (collapse succeeded)"
tui_send Space
tui_wait_for "▼ itest-fleet" 5

# Step 5: press k once from header — should wrap up to settings row.
info "pressing k once to wrap up to settings row"
tui_send k
sleep 0.2
tui_send Enter
tui_wait_for "Tmux vim keys" 5
info "k wrapped up to settings row (settings page rendered)"

# Step 6: return to the fleet list.
tui_send Escape
tui_wait_for "alpha" 5

pass "TUI cursor navigation wraps with j/k"
