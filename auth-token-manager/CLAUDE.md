# CLAUDE.md - Auth Token Manager

## About This Service

> Snapshot of the current CLI commands, health logic, and K8s secret workflow lives in `docs/auth-token-manager.md`.

This is a Deno-based microservice responsible for OAuth2 token management, specifically for refreshing Inoreader API tokens. It is built with a Test-Driven Development (TDD) first approach, ensuring reliability and security.

- **Runtime**: Deno 2.x with TypeScript
- **Core Task**: Inoreader OAuth2 token refresh and secure storage.

## TDD in Deno

All development must follow the Red-Green-Refactor TDD cycle. We use Deno's built-in testing tools for this.

### TDD Workflow: Refreshing a Token

Here is a step-by-step example of how to implement the token refresh logic using TDD.

**1. RED: Write a failing test.**

First, we test the `refreshToken` function, which doesn't exist yet. We use `stub` from `@std/testing/mock` to mock the `fetch` call, preventing real network requests.

```typescript
// tests/auth/refreshToken.test.ts
import { assertEquals, assertRejects } from "@std/testing/asserts";
import { stub } from "@std/testing/mock";
import { refreshToken } from "../../src/auth/refreshToken.ts";

Deno.test("refreshToken should return new tokens on success", async () => {
  // Mock a successful fetch response
  const fetchStub = stub(globalThis, "fetch", () =>
    Promise.resolve(
      new Response(JSON.stringify({ access_token: "new_token" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    )
  );

  try {
    const tokens = await refreshToken("old_refresh_token");
    assertEquals(tokens.access_token, "new_token");
  } finally {
    fetchStub.restore(); // Always restore the original function
  }
});
```

Running `deno test` will fail because `refreshToken` is not implemented.

**2. GREEN: Write the minimal code to pass.**

Now, we implement the function with the simplest logic to make the test pass.

```typescript
// src/auth/refreshToken.ts
export async function refreshToken(token: string): Promise<any> {
    const response = await fetch("https://www.inoreader.com/oauth2/token", {
        method: "POST",
        headers: {
            "Content-Type": "application/x-www-form-urlencoded",
        },
        body: new URLSearchParams({
            client_id: Deno.env.get("INOREADER_CLIENT_ID")!,
            client_secret: Deno.env.get("INOREADER_CLIENT_SECRET")!,
            grant_type: "refresh_token",
            refresh_token: token,
        }),
    });

    if (!response.ok) {
        throw new Error("Failed to refresh token");
    }

    return await response.json();
}
```

**3. REFACTOR: Improve the implementation.**

Refactor the code for clarity and robustness. The tests should still pass.

### BDD-Style Testing

For better organization, use `describe` and `it` from `@std/testing/bdd`.

```typescript
import { describe, it, afterEach } from "@std/testing/bdd";

describe("refreshToken()", () => {
    afterEach(() => {
        // Clean up mocks here
    });

    it("should return new tokens on success", async () => {
        // ... test logic
    });

    it("should throw an error on failure", async () => {
        // ... test logic for failure case
    });
});
```

## Secure Logging

**Critical**: Never log OAuth tokens or other credentials. Use a sanitized logger that redacts sensitive fields.

```typescript
import { logger } from "./utils/logger.ts";

// This will be automatically sanitized in the logs.
logger.info("Token refresh successful", {
  access_token: tokens.access_token, 
  refresh_token: tokens.refresh_token,
});
```

## References

-   [Deno Manual: Testing](https://deno.com/manual@v1.40/testing)
-   [Deno Standard Library: Mocking](https://deno.land/std@0.224.0/testing/mock.ts)
-   [Deno Standard Library: BDD](https://deno.land/std@0.224.0/testing/bdd.ts)
