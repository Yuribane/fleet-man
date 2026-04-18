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
    ├── 010_up_and_ls.sh
    ├── 020_exec.sh
    ├── 030_stop_start.sh
    ├── 040_status.sh
    ├── 050_down.sh
    └── 060_destroy.sh
```

## Contract

Every test file is standalone:

1. `source common.sh` at the top.
2. Call `setup_test` first — this wipes `~/.fleet` and re-creates a fresh
   git fixture repo at `${FIXTURE_REPO_DIR}`.
3. Drive the CLI with `"${FLEET_BIN}"` (set by `run.sh`) and assert with
   the `assert_*` helpers.
4. `exit 0` on success. `fail` helpers `exit 1`.

`run.sh` calls `teardown_test` after every test to nuke containers and
state so the next test starts clean.

## Prerequisites

- Go toolchain (for building `fleet`)
- Docker daemon reachable (`docker info` works)
- `devcontainer` CLI (`npm install -g @devcontainers/cli`)

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
