# auth-token-manager/CLAUDE.md

## Overview

OAuth2 token manager for Inoreader API. **Deno 2.x**, TypeScript.

> Details: `docs/services/auth-token-manager.md`

## Commands

```bash
# Test (TDD first)
deno test

# Coverage
deno test --coverage

# Run
deno task start

# Lint
deno fmt && deno lint
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- Use `@std/testing/asserts` for assertions
- Use `stub` from `@std/testing/mock` for mocking `fetch`
- Use `describe/it` from `@std/testing/bdd` for BDD-style tests

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **NEVER Log Secrets**: Use sanitized logger for tokens
3. **Restore Stubs**: ALWAYS restore mocked functions in `finally` blocks
4. **BDD Style**: Organize tests with `describe/it` blocks
