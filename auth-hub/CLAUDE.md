# auth-hub/CLAUDE.md

## Overview

Identity-Aware Proxy bridging Nginx `auth_request` with Ory Kratos. **Go 1.24+**.

> Details: `docs/services/auth-hub.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Run
go run main.go

# Health check
curl http://localhost:8888/health
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Unit**: Mock KratosClient and SessionCache
- **Integration**: Real Kratos instance
- **Table-driven**: Use for multiple scenarios

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Cache TTL**: 5 minutes (configurable via `CACHE_TTL`)
3. **NEVER Log Secrets**: Session tokens MUST NOT appear in logs
4. **Logging**: Use `log/slog` with JSON format
5. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
