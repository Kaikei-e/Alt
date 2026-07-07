---
name: clean-architecture
description: Alt project Clean Architecture layer patterns
---

# Clean Architecture Layers

```
Handler -> Usecase -> Port -> Gateway -> Driver
```

## Layer Rules

| Layer | Responsibility | Can Depend On |
|-------|----------------|---------------|
| Handler | HTTP/gRPC entry points, validation, response formatting | Usecase, Port |
| Usecase | Business logic orchestration, NO external dependencies | Port only |
| Port | Interface definitions (contracts) | Nothing |
| Gateway | Anti-corruption layer, external service mapping | Port, Driver |
| Driver | Database, API, external integrations | External libraries |

## File Patterns

- `**/rest/**` or `**/handler/**` = Handler layer
- `**/usecase/**` = Usecase layer
- `**/port/**` = Port layer (interfaces)
- `**/gateway/**` = Gateway layer
- `**/driver/**` = Driver layer

## Common Violations

Concrete patterns found repeatedly in the 2026-07 full-repo review — check for these before finishing any change:

- **Handler doing Driver work**: HTTP fetch, SSRF validation, or direct DB calls implemented inside a REST/RPC handler (seen as ~600 lines duplicated across 3 handlers). Handlers validate, delegate to a Usecase, and format the response — nothing else
- **Driver importing Service/Usecase** (reverse dependency): a `driver/` package importing from `service/` or `usecase/` inverts the layer direction
- **Usecase importing infrastructure**: `usecase/` importing `otel`, `httpx`, `asyncpg`, redis clients, or any `driver/` package directly — depend on a Port interface instead
- **Cross-layer duplication instead of extraction**: the same logic pasted into multiple handlers because it lives at the wrong layer — extract into a Usecase
- Circular dependencies between layers

## Self-Check (run before handoff)

```bash
# Go: usecase importing driver or otel directly
grep -rn --include="*.go" -E '"[^"]*/(driver|otel)' */app/usecase/ | grep -v _test

# Go: driver importing service/usecase (reverse dependency)
grep -rn --include="*.go" -E '"[^"]*/(service|usecase)' */app/driver/

# Python: usecase importing infrastructure
grep -rn --include="*.py" -E '^(from|import) .*(httpx|asyncpg|redis|driver)' */app/usecase/
```

For broad audits use the `layer-checker` agent.
