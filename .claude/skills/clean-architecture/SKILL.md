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

- Handler importing Driver directly (must go through Usecase)
- Usecase importing external packages (use Port interfaces)
- Circular dependencies between layers
