#!/usr/bin/env bash
# Verifies `fleet down` removes the container, wipes the workspace dir,
# and drops the instance from state.
set -euo pipefail

source "$(dirname "$0")/../common.sh"

setup_test
fleet_up alpha

container_id=$(grep -oE '"container_id":\s*"[^"]+"' "${HOME}/.fleet/state.json" | head -1 | sed -E 's/.*"([^"]+)"$/\1/')
# Fleet clones into <fleet>/<instance>/<fleet> so the leaf dir contains the
# workspace; `fleet down` wipes that leaf.
workspace_dir="${HOME}/.fleet/workspaces/${FIXTURE_REPO_NAME}/alpha/${FIXTURE_REPO_NAME}"
[ -n "${container_id}" ] || fail "could not read container_id"
assert_file_exists "${workspace_dir}"

info "fleet down alpha"
down_out=$("${FLEET_BIN}" down "${FIXTURE_REPO_NAME}/alpha")
printf '%s\n' "${down_out}"
assert_contains "${down_out}" "removed" "down should mention removed"

# Docker container gone.
if docker inspect "${container_id}" >/dev/null 2>&1; then
  fail "container ${container_id} still exists after down"
fi

# Workspace clone gone.
assert_file_absent "${workspace_dir}"

# State no longer has the instance.
state=$(cat "${HOME}/.fleet/state.json")
assert_not_contains "${state}" "\"name\": \"alpha\"" "instance should be removed from state"

# `ls` no longer lists it.
ls_out=$("${FLEET_BIN}" ls "${FIXTURE_REPO_NAME}")
assert_not_contains "${ls_out}" "alpha" "ls should not list removed instance"

pass "down"
