# fleet-man — Architecture & Tech Stack

## Tech Stack

- **Language:** Go
- **CLI framework:** [cobra](https://github.com/spf13/cobra)
- **TUI framework:** [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)
- **Underlying tool:** `devcontainer` CLI (npm package `@devcontainers/cli`)
- **State storage:** `$HOME/.fleet/` directory

## Project Structure

```
fleet-man/
├── cmd/
│   └── fleet/
│       └── main.go              # Entrypoint — wires up root command and runs
├── internal/
│   ├── cli/
│   │   ├── root.go              # Root cobra command (no args → TUI)
│   │   ├── up.go                # fleet up
│   │   ├── down.go              # fleet down
│   │   ├── list.go              # fleet list / fleet ls
│   │   ├── exec.go              # fleet exec
│   │   ├── code.go              # fleet code
│   │   ├── logs.go              # fleet logs
│   │   └── status.go            # fleet status
│   ├── tui/
│   │   ├── app.go               # Top-level bubbletea model
│   │   ├── fleet_view.go        # Fleet list view
│   │   ├── instance_view.go     # Instance detail/actions view
│   │   └── styles.go            # Lipgloss styles
│   ├── fleet/
│   │   ├── fleet.go             # Fleet type, fleet-level operations
│   │   ├── instance.go          # Instance type, lifecycle methods
│   │   └── resolve.go           # Fleet/instance name resolution (cwd, explicit, flag)
│   ├── state/
│   │   ├── state.go             # State struct, load/save from $HOME/.fleet/state.json
│   │   └── config.go            # Config struct, load/save from $HOME/.fleet/config.json
│   └── devcontainer/
│       ├── client.go            # Wraps devcontainer CLI calls (up, exec, etc.)
│       └── types.go             # Parsed output types from devcontainer CLI
├── go.mod
├── go.sum
└── plan/
    ├── overview.md
    └── architecture.md
```

## Entrypoint

Single binary, single entrypoint:

**`cmd/fleet/main.go`** — initializes the root cobra command and calls `Execute()`. The binary is named `fleet`.

```go
// cmd/fleet/main.go
package main

import (
    "os"
    "github.com/<org>/fleet-man/internal/cli"
)

func main() {
    if err := cli.NewRootCmd().Execute(); err != nil {
        os.Exit(1)
    }
}
```

## Internal Packages

### `internal/cli`

Cobra command definitions. Each file registers one subcommand on the root.

- **`root.go`** — Creates the root `fleet` command. When invoked with no args, launches the TUI. Registers all subcommands.
- **`up.go`** — `fleet up <name> [--repo <url>]`. Resolves fleet, calls `devcontainer.Client.Up()`, records instance in state.
- **`down.go`** — `fleet down <name>`. Stops and removes a container, removes from state.
- **`list.go`** — `fleet list [fleet]` / `fleet ls [fleet]`. Reads state, prints instance table.
- **`exec.go`** — `fleet exec <name> <cmd...>`. Looks up container ID from state, calls `devcontainer.Client.Exec()`.
- **`code.go`** — `fleet code <name>`. Opens VS Code attached to the container.
- **`logs.go`** — `fleet logs <name>`. Streams container logs.
- **`status.go`** — `fleet status`. Fleet-wide summary across all fleets.

### `internal/tui`

Bubbletea TUI application. Launched when `fleet` is run with no args.

- **`app.go`** — Root bubbletea model. Manages view switching and global key bindings.
- **`fleet_view.go`** — Lists fleets and their instances. Supports navigation and actions (up, down, exec, code).
- **`instance_view.go`** — Detail view for a single instance with action menu.
- **`styles.go`** — Shared lipgloss style definitions.

### `internal/fleet`

Core domain logic. No dependency on CLI or TUI.

- **`fleet.go`** — `Fleet` type (name, remote URL, instances). Methods for adding/removing instances.
- **`instance.go`** — `Instance` type (name, container ID, config path, timestamps, status). Lifecycle helpers.
- **`resolve.go`** — Resolves a user-provided name string into a fleet + instance. Handles:
  - Bare `<name>` → infer fleet from cwd git remote
  - `<fleet>/<name>` → look up fleet by name in state
  - `--repo` flag → find or create fleet by remote URL

### `internal/state`

Persistence layer. Reads/writes `$HOME/.fleet/`.

- **`state.go`** — `State` struct containing all fleets and instances. `Load()` / `Save()` to `$HOME/.fleet/state.json`. File locking for concurrent access.
- **`config.go`** — `Config` struct for user preferences. `Load()` / `Save()` to `$HOME/.fleet/config.json`.

### `internal/devcontainer`

Adapter for the `devcontainer` CLI. All subprocess calls go through here.

- **`client.go`** — `Client` struct with methods:
  - `Up(workspace, config)` — runs `devcontainer up`, returns container ID
  - `Exec(containerID, cmd)` — runs `devcontainer exec`
  - `Down(containerID)` — stops/removes the container
  - `ReadConfig(workspace)` — reads and parses devcontainer.json
- **`types.go`** — Go structs for parsing `devcontainer` CLI JSON output.

## Architecture Diagram

```
┌─────────────────────────────────────────────┐
│                cmd/fleet/main.go             │
└──────────────────────┬──────────────────────┘
                       │
┌──────────────────────▼──────────────────────┐
│              internal/cli                    │
│  root.go · up.go · down.go · list.go · ...  │
└──────┬───────────────────────────┬──────────┘
       │ (no args)                 │
┌──────▼──────┐          ┌────────▼───────────┐
│ internal/tui│          │  internal/fleet     │
│  app.go     │─────────▶│  fleet.go           │
│  views...   │          │  instance.go        │
└─────────────┘          │  resolve.go         │
                         └────────┬────────────┘
                                  │
                    ┌─────────────┼─────────────┐
                    │             │              │
             ┌──────▼─────┐ ┌────▼─────────┐    │
             │internal/    │ │internal/     │    │
             │state        │ │devcontainer  │    │
             │ state.go    │ │ client.go    │    │
             │ config.go   │ │ types.go     │    │
             └─────────────┘ └──────────────┘    │
```

## State Management

All state and config lives under `$HOME/.fleet/`:

```
$HOME/.fleet/
├── config.json          # User preferences
└── state.json           # Fleet and instance tracking
```

### `state.json`

```json
{
  "fleets": {
    "fleet-man": {
      "remote": "git@github.com:org/fleet-man.git",
      "instances": [
        {
          "name": "agent-1",
          "container_id": "abc123...",
          "config": ".devcontainer/devcontainer.json",
          "created_at": "2026-03-25T10:00:00Z",
          "status": "running"
        }
      ]
    }
  }
}
```

## Dependencies

| Module | Purpose |
|--------|---------|
| `github.com/spf13/cobra` | CLI command structure |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/bubbles` | TUI components (table, spinner, etc.) |

## Build

```bash
go build -o fleet ./cmd/fleet
```

Produces a single `fleet` binary.
