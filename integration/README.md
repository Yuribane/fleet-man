# fleet-man integration tests

End-to-end tests exercising the `fleet` CLI against the devcontainer
backend. Each test drives the real binary against a fixture repo and a
real docker daemon.

## Layout

```
integration/
├── run.sh              # driver: builds binary, runs every test, teardown between
├── common.sh           # shared helpers: assertions, setup/teardown, fleet_up
├── fixture/            # minimal devcontainer fixture cloned via file:// per test
│   └── .devcontainer/
│       └── devcontainer.json
└── tests/
    ├── 010_up_and_ls.sh        # CLI — up + ls
    ├── 020_exec.sh             # CLI — exec
    ├── 030_stop_start.sh       # CLI — stop / start
    ├── 040_status.sh           # CLI — status
    ├── 050_down.sh             # CLI — down
    ├── 060_destroy.sh          # CLI — destroy
    ├── 100_tui_startup.sh      # TUI — launch + render
    ├── 110_tui_collapse.sh     # TUI — space collapses / expands fleet
    ├── 120_tui_stop_start.sh   # TUI — s toggles instance status
    ├── 130_tui_delete_cancel.sh# TUI — d opens dialog, n cancels
    ├── 140_tui_quit.sh         # TUI — q exits cleanly
    └── 150_tui_refresh.sh      # TUI — r re-reads state
```

TUI tests drive the real `fleet` binary inside a detached `tmux` session
using `send-keys` and read back rendered state via `capture-pane` —
see `common.sh` `tui_*` helpers.

## Contract

Every test file is standalone:

1. Declare a short description in the header as `# Description: <text>` —
   `run.sh` greps this out and includes it in the Actions step summary.
2. `source common.sh` at the top.
3. Call `setup_test` first — this wipes `~/.fleet` and re-creates a fresh
   git fixture repo at `${FIXTURE_REPO_DIR}`.
4. Drive the CLI with `"${FLEET_BIN}"` (set by `run.sh`) and assert with
   the `assert_*` helpers.
5. `exit 0` on success. `fail` helpers `exit 1`.

`run.sh` calls `teardown_test` after every test to nuke containers and
state so the next test starts clean.

## Prerequisites

- Go toolchain (for building `fleet`)
- Docker daemon reachable (`docker info` works)
- `devcontainer` CLI (`npm install -g @devcontainers/cli`)
- `tmux` (used by the TUI tests as a headless terminal)

## Running

```
./integration/run.sh
```

Use a prebuilt binary:

```
FLEET_BIN=/path/to/fleet ./integration/run.sh
```

## CI

`.github/workflows/integration.yml` runs this suite on every PR to
`main`.
