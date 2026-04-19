#!/usr/bin/env bash
# Description: Pressing `s` on a running instance toggles its status to stopped and back.
set -euo pipefail

source "$(dirname "$0")/../common.sh"

trap 'tui_kill' EXIT

setup_test
fleet_up alpha

tui_spawn
tui_wait_for "running" 15

info "Move cursor down to the alpha instance"
tui_send j
sleep 0.5

info "Press 's' to stop"
tui_send s
# Message shown below the list on completion.
tui_wait_for "Stopped itest-fleet/alpha" 20
tui_wait_for "stopped" 5

info "Press 's' again to start"
tui_send s
tui_wait_for "Started itest-fleet/alpha" 20
tui_wait_for "running" 5

pass "TUI stop / start via s"
