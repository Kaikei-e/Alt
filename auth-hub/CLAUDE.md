# auth-hub/CLAUDE.md

## Overview

Translates an Ory Kratos session cookie into Alt's internal backend JWT
(`X-Alt-Backend-Token`). Also enrolls its own east-west mTLS leaf in-process
(one of the 14 [[000978]] parents). **Go 1.26+**.

> Details: `docs/services/auth-hub.md`

The `/validate` endpoint was originally built for nginx `auth_request`, but
when the edge proxy moved to PlectoProxy (`plecto-proxy`,
`plecto/manifest.toml`), `/auth-validate` was deliberately not ported (see
the manifest's own comment on that route). `nginx/conf.d/default.conf`
survives in the repo only as an unloaded prototype config — no compose
service reads it. The live path today is alt-frontend-sv calling `/session`
directly and forwarding the returned JWT; alt-backend / alt-butterfly-facade
verify it in-process against `backend_token_secret`.

## Architecture

Clean Architecture with domain-driven layers. `cmd/auth-hub` is the only
entry point (the old flat `main.go` + `handler/`/`cache/`/`client/`/`token/`
packages were deleted as an unused, unauthenticated second entrypoint):

```
cmd/auth-hub/main.go           # Entry point + DI wiring (errgroup graceful shutdown)
internal/
  domain/                       # Entities, errors, port interfaces (zero deps)
  usecase/                      # Business logic (validate, session, csrf, system-user)
  adapter/handler/              # HTTP handlers (Echo) + error mapper
  adapter/gateway/              # Kratos client (domain.SessionValidator, IdentityProvider)
  infrastructure/cache/         # Session cache (domain.SessionCache)
  infrastructure/token/         # JWT + CSRF generators (domain.TokenIssuer, CSRFTokenGenerator)
  pki/                          # In-process east-west mTLS leaf enrollment/renewal (pki.Start)
tlsutil/                        # TLS config for the optional mTLS listener (:9443, MTLS_LISTEN=true)
middleware/                     # Security headers, rate limiting, internal auth, OTel
config/                         # Configuration loading + validation
```

## Commands

```bash
# Test (TDD first)
go test ./...
go test ./... -race             # With race detector

# Run
go run ./cmd/auth-hub

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
