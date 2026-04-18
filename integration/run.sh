#!/usr/bin/env bash
# Integration test driver for fleet-man.
#
# - Builds the fleet binary to a temp location (unless FLEET_BIN is set).
# - Runs every test in integration/tests in alphabetical order.
# - Between tests: runs teardown to kill any leftover containers / state.
# - Exits 0 only if every test passes.

set -euo pipefail

INTEGRATION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${INTEGRATION_DIR}/.." && pwd)"

# Colors
if [ -t 1 ]; then
  BOLD='\033[1m'; GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; RESET='\033[0m'
else
  BOLD=''; GREEN=''; RED=''; YELLOW=''; RESET=''
fi

say() { printf "%b==>%b %s\n" "${BOLD}" "${RESET}" "$*"; }

# 1. Build fleet binary unless caller provided one.
if [ -z "${FLEET_BIN:-}" ]; then
  build_dir="$(mktemp -d)"
  FLEET_BIN="${build_dir}/fleet"
  say "Building fleet -> ${FLEET_BIN}"
  (cd "${REPO_ROOT}" && go build -o "${FLEET_BIN}" ./cmd/fleet)
fi
export FLEET_BIN
say "Using fleet binary: ${FLEET_BIN}"
"${FLEET_BIN}" --help >/dev/null || { printf "%b[fatal]%b fleet binary failed to run\n" "${RED}" "${RESET}" >&2; exit 2; }

# 2. Preflight: docker + devcontainer CLI must be available and usable.
if ! command -v docker >/dev/null 2>&1; then
  printf "%b[fatal]%b docker CLI not on PATH\n" "${RED}" "${RESET}" >&2
  exit 2
fi
if ! docker info >/dev/null 2>&1; then
  printf "%b[fatal]%b docker daemon not reachable\n" "${RED}" "${RESET}" >&2
  exit 2
fi
if ! command -v devcontainer >/dev/null 2>&1; then
  printf "%b[fatal]%b devcontainer CLI not on PATH (npm i -g @devcontainers/cli)\n" "${RED}" "${RESET}" >&2
  exit 2
fi

# 3. Pre-pull the fixture image once so each test is not waiting on image pulls.
fixture_image=$(grep -oE '"image":\s*"[^"]+"' "${INTEGRATION_DIR}/fixture/.devcontainer/devcontainer.json" | sed -E 's/.*"([^"]+)"$/\1/')
if [ -n "${fixture_image}" ]; then
  say "Pre-pulling fixture image: ${fixture_image}"
  docker pull -q "${fixture_image}" >/dev/null || true
fi

# 4. Iterate tests.
shopt -s nullglob
tests=("${INTEGRATION_DIR}"/tests/*.sh)
shopt -u nullglob
if [ "${#tests[@]}" -eq 0 ]; then
  printf "%b[fatal]%b no tests found in %s\n" "${RED}" "${RESET}" "${INTEGRATION_DIR}/tests" >&2
  exit 2
fi

passed=0
failed=0
failed_tests=()

# Sourced common.sh in tests provides teardown_test — we need it here too.
# shellcheck disable=SC1091
source "${INTEGRATION_DIR}/common.sh"

for test_file in "${tests[@]}"; do
  test_name=$(basename "${test_file}" .sh)
  printf "\n"
  say "${BOLD}RUN${RESET}  ${test_name}"
  t_start=$(date +%s)
  if bash "${test_file}"; then
    t_end=$(date +%s)
    printf "%b==>%b %bPASS%b ${test_name} (%ds)\n" "${BOLD}" "${RESET}" "${GREEN}" "${RESET}" "$((t_end - t_start))"
    passed=$((passed + 1))
  else
    t_end=$(date +%s)
    printf "%b==>%b %bFAIL%b ${test_name} (%ds)\n" "${BOLD}" "${RESET}" "${RED}" "${RESET}" "$((t_end - t_start))"
    failed=$((failed + 1))
    failed_tests+=("${test_name}")
  fi

  say "teardown ${test_name}"
  teardown_test || true
done

printf "\n%b==>%b Summary: %b%d passed%b, %b%d failed%b\n" \
  "${BOLD}" "${RESET}" \
  "${GREEN}" "${passed}" "${RESET}" \
  "${RED}"   "${failed}" "${RESET}"

if [ "${failed}" -gt 0 ]; then
  printf "Failed: %s\n" "${failed_tests[*]}"
  exit 1
fi
