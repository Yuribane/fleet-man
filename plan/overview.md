# fleet-man — Project Overview

## What

`fleet-man` is a local CLI/TUI tool (invoked as `fleet`) that wraps the `devcontainer` CLI to make it easy to manage fleets of devcontainers spawned from a repo.

## Why

When running multiple AI agents in parallel, each in its own devcontainer, managing those containers becomes tedious. `fleet` provides a single interface to spawn, name, list, exec into, attach VS Code to, and tear down devcontainers.

## Core Concepts

- **Fleet** — The set of devcontainer instances spawned from a repo. A fleet is identified by the git remote URL of the repo.
- **Instance** — A single running devcontainer within a fleet, identified by a user-friendly name.

### Instance Addressing

Instances are addressed as `<fleet>/<instance>` (e.g. `fleet-man/agent-1`). The fleet portion can be omitted when the cwd is inside a git repo — the tool will match the cwd repo's remote URL to determine the fleet automatically.

```bash
# Inside the fleet-man repo — fleet is inferred
fleet up agent-1
fleet exec agent-1 bash

# Explicit fleet — works from anywhere
fleet up fleet-man/agent-1
fleet exec fleet-man/agent-1 bash

# Manage a different repo's fleet from here
fleet ls other-project
```

## CLI Command: `fleet`

### Subcommands

| Command | Description |
|---------|-------------|
| `fleet up <name>` | Spawn a new devcontainer instance (see [fleet up details](#fleet-up)) |
| `fleet down <name>` | Stop and remove a devcontainer instance |
| `fleet list [fleet]` / `fleet ls [fleet]` | List instances in a fleet (defaults to current repo's fleet) |
| `fleet exec <name> <cmd>` | Execute a command inside a named instance |
| `fleet code <name>` | Open VS Code attached to the named instance |
| `fleet logs <name>` | Show logs for an instance |
| `fleet status` | Show fleet-wide status summary |
| `fleet` (no args) | Launch interactive TUI for managing the fleet |

### fleet up

`fleet up <name>` resolves the repo to clone in this order:

1. **Inside a git repo (cwd)** — uses the cwd repo's remote origin. The fleet name is inferred.
2. **`--repo <git-url>` flag** — explicitly provide a git URL. A new fleet is created if one doesn't exist for that remote.
3. **`<fleet>/<name>` addressing** — references an existing fleet by name, reusing the repo URL already recorded for that fleet.

```bash
# Inside a repo — infers remote origin
fleet up agent-1

# From anywhere — provide a git URL
fleet up agent-1 --repo git@github.com:org/my-project.git

# From anywhere — reference existing fleet
fleet up my-project/agent-1
```

### Example Workflow

```bash
# Spawn 3 agent containers from current repo
fleet up agent-1
fleet up agent-2
fleet up agent-3

# Check what's running
fleet ls

# Exec into one
fleet exec agent-1 bash

# Open VS Code on another
fleet code agent-2

# Tear one down
fleet down agent-3
```

See [architecture.md](architecture.md) for tech stack, project structure, and state management details.

