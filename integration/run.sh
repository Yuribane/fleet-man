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
# Per-test result rows for the step summary: "<name>|<status>|<duration>s|<description>"
result_rows=()

# Pull a short test description out of a "# Description: ..." header line,
# or fall back to "(no description)" if the file does not declare one.
test_description() {
  local file="$1"
  local desc
  desc=$(grep -m1 -E '^# Description: *' "${file}" 2>/dev/null | sed -E 's/^# Description: *//')
  if [ -z "${desc}" ]; then
    echo "(no description)"
  else
    echo "${desc}"
  fi
}

# Sourced common.sh in tests provides teardown_test — we need it here too.
# shellcheck disable=SC1091
source "${INTEGRATION_DIR}/common.sh"

for test_file in "${tests[@]}"; do
  test_name=$(basename "${test_file}" .sh)
  test_desc=$(test_description "${test_file}")
  printf "\n"
  say "${BOLD}RUN${RESET}  ${test_name} — ${test_desc}"
  t_start=$(date +%s)
  if bash "${test_file}"; then
    t_end=$(date +%s)
    dur=$((t_end - t_start))
    printf "%b==>%b %bPASS%b ${test_name} (%ds)\n" "${BOLD}" "${RESET}" "${GREEN}" "${RESET}" "${dur}"
    passed=$((passed + 1))
    result_rows+=("${test_name}|pass|${dur}|${test_desc}")
  else
    t_end=$(date +%s)
    dur=$((t_end - t_start))
    printf "%b==>%b %bFAIL%b ${test_name} (%ds)\n" "${BOLD}" "${RESET}" "${RED}" "${RESET}" "${dur}"
    failed=$((failed + 1))
    failed_tests+=("${test_name}")
    result_rows+=("${test_name}|fail|${dur}|${test_desc}")
  fi

  say "teardown ${test_name}"
  teardown_test || true
done

printf "\n%b==>%b Summary: %b%d passed%b, %b%d failed%b\n" \
  "${BOLD}" "${RESET}" \
  "${GREEN}" "${passed}" "${RESET}" \
  "${RED}"   "${failed}" "${RESET}"

# Emit a GitHub Actions step summary if running inside a workflow.
# GITHUB_STEP_SUMMARY is a file path the runner publishes as markdown on
# the job summary page. Outside Actions this env var is unset and we skip.
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  total=$((passed + failed))
  if [ "${failed}" -eq 0 ]; then
    overall="✅ all green"
  else
    overall="❌ ${failed} failing"
  fi
  {
    printf '## Integration Tests — %s\n\n' "${overall}"
    printf '**%d passed**, **%d failed** out of %d.\n\n' "${passed}" "${failed}" "${total}"
    printf '| Test | Status | Duration | Description |\n'
    printf '| --- | --- | ---: | --- |\n'
    for row in "${result_rows[@]}"; do
      IFS='|' read -r name status dur desc <<< "${row}"
      if [ "${status}" = "pass" ]; then
        icon="✅ pass"
      else
        icon="❌ fail"
      fi
      printf '| `%s` | %s | %ss | %s |\n' "${name}" "${icon}" "${dur}" "${desc}"
    done
    if [ "${failed}" -gt 0 ]; then
      printf '\n**Failed:** %s\n' "${failed_tests[*]}"
    fi
  } >> "${GITHUB_STEP_SUMMARY}"
fi

if [ "${failed}" -gt 0 ]; then
  printf "Failed: %s\n" "${failed_tests[*]}"
  exit 1
fi
