# altctl - Alt Platform Orchestration CLI

## Overview

`altctl` is a Go CLI tool for managing the Alt platform's Docker Compose services through stack-based orchestration with automatic dependency resolution.

## Architecture

```
altctl/
├── main.go                     # Entry point
├── cmd/                        # Cobra commands
│   ├── root.go                 # Root command, global flags, Viper init
│   ├── up.go                   # Start stacks
│   ├── down.go                 # Stop stacks
│   ├── status.go               # Show service status
│   ├── list.go                 # List available stacks
│   ├── logs.go                 # Stream service logs
│   ├── build.go                # Build images
│   ├── config.go               # Show configuration
│   ├── completion.go           # Shell completions
│   └── version.go              # Version info
├── internal/
│   ├── config/                 # Viper configuration
│   │   └── config.go           # Config loading and validation
│   ├── stack/                  # Stack definitions
│   │   ├── stack.go            # Stack type
│   │   ├── registry.go         # Stack registry
│   │   └── dependency.go       # Dependency resolver
│   ├── compose/                # Docker Compose operations
│   │   ├── client.go           # Compose client
│   │   └── executor.go         # Command executor
│   └── output/                 # CLI output formatting
│       ├── printer.go          # Message printing
│       └── table.go            # Table rendering
└── go.mod
```

## Stack Definitions

| Stack | Services | Dependencies | Optional |
|-------|----------|--------------|----------|
| base | (shared resources) | - | No |
| db | db, meilisearch, clickhouse | base | No |
| auth | kratos-db, kratos-migrate, kratos, auth-hub | base | No |
| core | nginx, alt-frontend, alt-frontend-sv, alt-backend, migrate | base, db, auth | No |
| ai | news-creator, news-creator-volume-init, pre-processor | base, db, core | Yes (GPU) |
| workers | pre-processor-sidecar, search-indexer, tag-generator, auth-token-manager | base, db, core | No |
| recap | recap-db, recap-db-migrator, recap-worker, recap-subworker, dashboard, recap-evaluator | base, db, core | Yes |
| logging | rask-log-aggregator, *-logs forwarders | base, db | Yes |
| rag | rag-db, rag-db-migrator, rag-orchestrator | base, db, core, workers | Yes |

## Usage

```bash
# Start default stacks (db, auth, core, workers)
altctl up

# Start specific stacks (dependencies auto-resolved)
altctl up ai                    # Starts: base → db → auth → core → ai
altctl up recap rag             # Starts both with all dependencies

# Start all stacks
altctl up --all

# Build and start
altctl up core --build

# Stop stacks
altctl down                     # Stop all
altctl down ai                  # Stop AI and dependents
altctl down --volumes           # Stop and remove volumes

# View status
altctl status                   # Table view
altctl status --json            # JSON output
altctl status --watch           # Live updates

# View logs
altctl logs alt-backend -f      # Follow logs
altctl logs db -n 200           # Last 200 lines

# List stacks
altctl list                     # Basic list
altctl list --services          # Include service details
altctl list --deps              # Show dependency graph

# Configuration
altctl config                   # Show current config
altctl config --json            # JSON output

# Shell completion
altctl completion bash > /etc/bash_completion.d/altctl
```

## Configuration

Configuration is loaded from `.altctl.yaml` in the project root or `~/.config/altctl/`.

```yaml
project:
  root: "/home/USER_NAME/Alt"
  docker_context: "default"

compose:
  dir: "compose"
  base_file: "base.yaml"

defaults:
  stacks: [db, auth, core, workers]

stacks:
  ai:
    requires_gpu: true
    startup_timeout: 600s

logging:
  level: "info"
  format: "text"

output:
  colors: true
  progress: true
```

## Development

```bash
# Build
make build

# Run tests
make test

# Format code
make fmt

# Install locally
make install-local
```

## Testing

Run tests with:
```bash
cd altctl && go test ./...
```

## Dependencies

- Go 1.25+
- github.com/spf13/cobra v1.8.1
- github.com/spf13/viper v1.19.0
- github.com/fatih/color v1.18.0
- github.com/olekukonko/tablewriter v0.0.5
