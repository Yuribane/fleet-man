#!/usr/bin/env bash
# Shared helpers for fleet-man integration tests.
#
# Each test file sources this, calls `setup_test`, runs assertions, and
# exits 0 on pass. `run.sh` invokes teardown between tests so state does
# not bleed across runs.

set -euo pipefail

# Colors (only when stdout is a tty)
if [ -t 1 ]; then
  C_RED='\033[0;31m'
  C_GREEN='\033[0;32m'
  C_YELLOW='\033[0;33m'
  C_RESET='\033[0m'
else
  C_RED=''
  C_GREEN=''
  C_YELLOW=''
  C_RESET=''
fi

# Paths resolved relative to this file's directory.
INTEGRATION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${INTEGRATION_DIR}/.." && pwd)"
FIXTURE_SRC="${INTEGRATION_DIR}/fixture"

# Test scratch dir created fresh per test.
TEST_SCRATCH_DIR="${TEST_SCRATCH_DIR:-/tmp/fleet-man-itest}"

# The fleet binary under test. run.sh builds it and exports FLEET_BIN.
FLEET_BIN="${FLEET_BIN:-fleet}"

# Fleet name is derived from the repo URL basename. We clone the fixture
# into a dir named "itest-fleet" so resolve.go sees fleet "itest-fleet".
FIXTURE_REPO_NAME="itest-fleet"
FIXTURE_REPO_DIR="${TEST_SCRATCH_DIR}/${FIXTURE_REPO_NAME}"
FIXTURE_REPO_URL="file://${FIXTURE_REPO_DIR}"

# === Logging ===

info()  { printf "%b[info]%b  %s\n" "${C_YELLOW}" "${C_RESET}" "$*"; }
pass()  { printf "%b[pass]%b  %s\n" "${C_GREEN}"  "${C_RESET}" "$*"; }
fail()  { printf "%b[fail]%b  %s\n" "${C_RED}"    "${C_RESET}" "$*" >&2; exit 1; }

# === Assertions ===

# assert_equals <expected> <actual> <message>
assert_equals() {
  local expected="$1"; local actual="$2"; local msg="${3:-values not equal}"
  if [ "$expected" != "$actual" ]; then
    fail "${msg}: expected=[${expected}] actual=[${actual}]"
  fi
}

# assert_contains <haystack> <needle> <message>
assert_contains() {
  local haystack="$1"; local needle="$2"; local msg="${3:-substring not found}"
  if ! printf '%s' "${haystack}" | grep -qF -- "${needle}"; then
    fail "${msg}: needle=[${needle}] haystack=[${haystack}]"
  fi
}

# assert_not_contains <haystack> <needle> <message>
assert_not_contains() {
  local haystack="$1"; local needle="$2"; local msg="${3:-substring unexpectedly found}"
  if printf '%s' "${haystack}" | grep -qF -- "${needle}"; then
    fail "${msg}: needle=[${needle}] haystack=[${haystack}]"
  fi
}

# assert_file_exists <path>
assert_file_exists() {
  [ -e "$1" ] || fail "expected path to exist: $1"
}

# assert_file_absent <path>
assert_file_absent() {
  [ ! -e "$1" ] || fail "expected path to be absent: $1"
}

# === Setup / teardown ===

# setup_test prepares a clean environment:
#   1. removes ~/.fleet
#   2. recreates the fixture git repo at $FIXTURE_REPO_DIR
# Called at the top of each test.
setup_test() {
  # 1. Wipe fleet state.
  rm -rf "${HOME}/.fleet"

  # 2. Re-create the fixture repo from the committed fixture template.
  rm -rf "${TEST_SCRATCH_DIR}"
  mkdir -p "${FIXTURE_REPO_DIR}"
  cp -r "${FIXTURE_SRC}/." "${FIXTURE_REPO_DIR}/"

  (
    cd "${FIXTURE_REPO_DIR}"
    git init -q -b main
    git config user.email "itest@fleet-man.local"
    git config user.name  "Fleet Integration Test"
    git add -A
    git commit -q -m "fixture: initial commit"
  )
}

# teardown_test best-effort cleans up any containers spawned by a test.
# It tears down every fleet in state, then wipes state files. Called by
# run.sh after each test whether it passed or failed.
teardown_test() {
  # Tear down containers. Best effort — a test that never reached
  # "up" will have no fleets; a test that left state in a weird place
  # is recovered by the fallback docker sweep below.
  if [ -f "${HOME}/.fleet/state.json" ]; then
    # Iterate fleet names out of state.json without jq (not guaranteed on
    # the runner). Grep the top-level keys under "fleets".
    local fleets
    fleets=$(grep -oE '"[a-zA-Z0-9._-]+":\s*\{' "${HOME}/.fleet/state.json" 2>/dev/null \
      | grep -v '"fleets"' \
      | sed -E 's/"([^"]+)".*/\1/' \
      | sort -u || true)
    for f in ${fleets}; do
      "${FLEET_BIN}" destroy "${f}" >/dev/null 2>&1 || true
    done
  fi

  # Fallback: remove any devcontainer created from the fixture workspace.
  # The devcontainer CLI labels containers with the workspace folder path.
  if command -v docker >/dev/null 2>&1; then
    local stale
    stale=$(docker ps -aq --filter "label=devcontainer.local_folder=${HOME}/.fleet/workspaces" 2>/dev/null || true)
    if [ -n "${stale}" ]; then
      docker rm -f ${stale} >/dev/null 2>&1 || true
    fi
  fi

  rm -rf "${HOME}/.fleet"
  rm -rf "${TEST_SCRATCH_DIR}"
}

# fleet_up spawns an instance against the fixture repo. Uses --repo so
# the test does not depend on cwd being a git repo. Streams output to
# the caller's stdout/stderr.
fleet_up() {
  local name="$1"
  "${FLEET_BIN}" up "${name}" --repo "${FIXTURE_REPO_URL}"
}
