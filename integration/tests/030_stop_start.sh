#!/usr/bin/env bash
# Verifies the stop -> start lifecycle: status transitions are persisted
# and the underlying docker container is actually stopped/started.
set -euo pipefail

source "$(dirname "$0")/../common.sh"

setup_test
fleet_up alpha

# Grab the container ID from state so we can independently verify docker state.
container_id=$(grep -oE '"container_id":\s*"[^"]+"' "${HOME}/.fleet/state.json" | head -1 | sed -E 's/.*"([^"]+)"$/\1/')
[ -n "${container_id}" ] || fail "could not read container_id from state.json"
info "container id: ${container_id}"

# --- stop ---
info "fleet stop alpha"
stop_out=$("${FLEET_BIN}" stop "${FIXTURE_REPO_NAME}/alpha")
printf '%s\n' "${stop_out}"
assert_contains "${stop_out}" "stopped" "stop output should mention stopped"

# docker reports the container as exited/created, not running.
docker_state=$(docker inspect -f '{{.State.Status}}' "${container_id}")
info "docker state after stop: ${docker_state}"
if [ "${docker_state}" = "running" ]; then
  fail "expected container to not be running after stop, got: ${docker_state}"
fi

ls_after_stop=$("${FLEET_BIN}" ls "${FIXTURE_REPO_NAME}")
assert_contains "${ls_after_stop}" "stopped" "ls should report stopped status"

# Second stop is a no-op.
info "fleet stop alpha (already stopped)"
stop_again=$("${FLEET_BIN}" stop "${FIXTURE_REPO_NAME}/alpha")
assert_contains "${stop_again}" "already stopped" "repeated stop should be a no-op"

# --- start ---
info "fleet start alpha"
start_out=$("${FLEET_BIN}" start "${FIXTURE_REPO_NAME}/alpha")
printf '%s\n' "${start_out}"
assert_contains "${start_out}" "started" "start output should mention started"

docker_state=$(docker inspect -f '{{.State.Status}}' "${container_id}")
assert_equals "running" "${docker_state}" "container should be running after start"

ls_after_start=$("${FLEET_BIN}" ls "${FIXTURE_REPO_NAME}")
assert_contains "${ls_after_start}" "running" "ls should report running status"

# exec works again after start.
echo_out=$("${FLEET_BIN}" exec "${FIXTURE_REPO_NAME}/alpha" -- sh -c "echo back-online")
assert_equals "back-online" "${echo_out}" "exec after start"

pass "stop + start"
