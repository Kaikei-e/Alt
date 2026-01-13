# auth-token-manager/CLAUDE.md

## Overview

Deno-based OAuth2 token manager for Inoreader API. Handles token refresh and secure storage. Built with **Deno 2.x** and TypeScript.

> For CLI commands and K8s secret workflow, see `docs/services/auth-token-manager.md`.

## Quick Start

```bash
# Run tests
deno test

# Run with coverage
deno test --coverage

# Start service
deno task start

# Format and lint
deno fmt && deno lint
```

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve quality, keep tests green

Testing with Deno:
- Use `@std/testing/asserts` for assertions
- Use `stub` from `@std/testing/mock` for mocking `fetch`
- Use `describe/it` from `@std/testing/bdd` for BDD-style tests

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Never Log Secrets**: Use sanitized logger for tokens
3. **Restore Stubs**: Always restore mocked functions in `finally` blocks
4. **Use BDD Style**: Organize tests with `describe/it` blocks

## Testing with Stubs

```typescript
import { stub } from "@std/testing/mock";

Deno.test("refreshToken returns new tokens", async () => {
  const fetchStub = stub(globalThis, "fetch", () =>
    Promise.resolve(new Response(JSON.stringify({ access_token: "new" })))
  );
  try {
    const tokens = await refreshToken("old_token");
    assertEquals(tokens.access_token, "new");
  } finally {
    fetchStub.restore();
  }
});
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Stub not restored | Use `finally` block |
| Token in logs | Use sanitized logger |
| Network call in tests | Mock `fetch` with stub |

## Appendix: References

### Official Documentation
- [Deno Testing](https://docs.deno.com/runtime/fundamentals/testing/)
- [Deno Standard Library: Mock](https://jsr.io/@std/testing/doc/mock/~)
- [Deno Standard Library: BDD](https://jsr.io/@std/testing/doc/bdd/~)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Deno Style Guide](https://docs.deno.com/runtime/contributing/style_guide/)
