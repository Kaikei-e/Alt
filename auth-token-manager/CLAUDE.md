# auth-token-manager/CLAUDE.md

## Overview

OAuth2 token manager for Inoreader API. **Deno 2.x**, TypeScript.

> Details: `docs/services/auth-token-manager.md`

## Architecture

Clean Architecture: `Handler -> Usecase -> Port -> Gateway`

```
main.ts                    # DI container (entry point)
src/
├── domain/types.ts        # Domain models & interfaces
├── port/                  # Interface definitions
│   ├── secret_manager.ts
│   ├── token_client.ts
│   └── http_client.ts
├── gateway/               # Port implementations
│   ├── env_file_secret_manager.ts
│   ├── inoreader_token_client.ts
│   └── fetch_http_client.ts
├── usecase/               # Business logic
│   ├── refresh_token.ts
│   ├── health_check.ts
│   ├── monitor_token.ts
│   └── authorize.ts
├── handler/               # CLI + HTTP handlers
│   ├── cli.ts
│   ├── oauth_server.ts
│   └── daemon.ts
└── infra/                 # Cross-cutting concerns
    ├── config.ts
    ├── logger.ts
    ├── otel.ts
    └── retry.ts
```

## Commands

```bash
# Test (TDD first)
deno test --allow-all

# Coverage
deno test --allow-all --coverage

# Run
deno task start

# Lint + Format
deno fmt && deno lint

# Type check
deno check main.ts
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- Use `@std/testing/asserts` for assertions
- Use `stub` from `@std/testing/mock` for mocking `fetch`
- Use `describe/it` from `@std/testing/bdd` for BDD-style tests
- Add `{ sanitizeResources: false, sanitizeOps: false }` to describe blocks
  (OTel timers)

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **NEVER Log Secrets**: Use sanitized logger for tokens
3. **Restore Stubs**: ALWAYS restore mocked functions in `finally` blocks
4. **BDD Style**: Organize tests with `describe/it` blocks
5. **Clean Architecture**: No import from inner layers to outer layers
