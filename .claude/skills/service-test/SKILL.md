---
name: service-test
description: Run one Alt service's test suite, detecting the language and the right runner from the files in its directory. Use when the user asks to test, re-test, or check a specific service by name. For the full pre-handoff sweep across every touched service — formatters, linters, type checkers, security scanners — use tdd-workflow Phase 5 instead, since a passing test suite is not a passing CI run.
allowed-tools: Bash, Read, Glob
argument-hint: <service-directory>
---

# Service Test

Run the test suite for the service named in `$ARGUMENTS`.

## Detection

| Language | Detection file | Command |
|----------|----------------|---------|
| Go | `go.mod` | `go test ./...` |
| Python | `pyproject.toml` | `uv run pytest` |
| Rust | `Cargo.toml` | `cargo test` |
| TypeScript (bun) | `package.json` + `bun.lockb` | `bun test` |
| Deno | `deno.json` | `deno test` |

## Steps

1. Locate the service directory. Most Go and Python services keep their module one level down, in
   `<service>/app/` — check there when the config file is not at the service root.
2. Detect the language from the config file present.
3. Run the matching command.
4. Report the result, including the failing test names when it is red.

Contract tests are excluded from these defaults because they need build tags, `--ignored`, or a
`SERVICE_SECRET`. Run them from `tdd-workflow`'s [cdc-map](../tdd-workflow/references/cdc-map.md)
when a service boundary is in scope.
