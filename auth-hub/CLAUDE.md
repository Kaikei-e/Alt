# auth-hub/CLAUDE.md

## Overview

Identity-Aware Proxy bridging Nginx `auth_request` with Ory Kratos. **Go 1.25+**.

> Details: `docs/services/auth-hub.md`

## Architecture

Clean Architecture with domain-driven layers:

```
cmd/auth-hub/main.go           # Entry point + DI wiring (errgroup graceful shutdown)
internal/
  domain/                       # Entities, errors, port interfaces (zero deps)
  usecase/                      # Business logic (validate, session, csrf, system-user)
  adapter/handler/              # HTTP handlers (Echo) + error mapper
  adapter/gateway/              # Kratos client (domain.SessionValidator, IdentityProvider)
  infrastructure/cache/         # Session cache (domain.SessionCache)
  infrastructure/token/         # JWT + CSRF generators (domain.TokenIssuer, CSRFTokenGenerator)
middleware/                     # Security headers, rate limiting, internal auth, OTel
config/                         # Configuration loading + validation
```

Legacy flat handlers in `handler/` are preserved for backward compatibility.

## Commands

```bash
# Test (TDD first)
go test ./...
go test ./... -race             # With race detector

# Run (new architecture)
go run ./cmd/auth-hub

# Run (legacy)
go run main.go

# Build
go build -o auth-hub ./cmd/auth-hub

# Health check
curl http://localhost:8888/health
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Unit**: Mock domain interfaces (SessionValidator, SessionCache, TokenIssuer)
- **Integration**: Real Kratos instance
- **Table-driven**: Use for multiple scenarios

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Cache TTL**: 5 minutes (configurable via `CACHE_TTL`)
3. **NEVER Log Secrets**: Session tokens MUST NOT appear in logs
4. **Logging**: Use `log/slog` with JSON format
5. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)` with domain sentinel errors
6. **Domain Errors**: Use `errors.Is()` with `internal/domain/errors.go` sentinels, not string matching
7. **Timing Safety**: Use `crypto/subtle.ConstantTimeCompare` for secret comparisons
8. **Rate Limiting**: All endpoints have IP-based rate limits via middleware
