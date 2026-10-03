/**
 * auth_fixture_test.ts
 *
 * Unit tests for validateFixtureTokenAtInit logic.
 *
 * Exercises the ACTUAL k6/helpers/auth.js helper loaded into a mocked VM context
 * (wiring getConfig + jwt.js decodeJWT) rather than copying the implementation.
 *
 * Run with:
 *   deno test --allow-read tests/unit/auth_fixture_test.ts
 */

import { assertEquals, assertThrows } from "@std/assert";
import { describe, it } from "@std/testing/bdd";
import * as fs from "node:fs";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const authPath = fileURLToPath(new URL("../../k6/helpers/auth.js", import.meta.url));
const jwtPath = fileURLToPath(new URL("../../k6/helpers/jwt.js", import.meta.url));

const jwtSource = fs.readFileSync(jwtPath, "utf-8").replace(/export\s+/g, "");
const authRaw = fs
  .readFileSync(authPath, "utf-8")
  .replace(/import\s*\{[^}]*getConfig[^}]*\}\s*from\s*["\x27][^"\x27]+["\x27];?/g, "")
  .replace(/import\s*\{[^}]*decodeJWT[^}]*\}\s*from\s*["\x27][^"\x27]+["\x27];?/g, "")
  .replace(/export\s+function\s+(\w+)/g, "function $1");
const authSource = `${authRaw}\nObject.assign(exports, { validateAuthHubToken, validateCohortTokenAtInit, validateFixtureTokenAtInit, getAuthHeaders, getPublicHeaders });`;

interface AuthModule {
  validateAuthHubToken: (
    token: string,
    cfg?: unknown,
    options?: unknown,
  ) => Record<string, unknown>;
  validateCohortTokenAtInit: (
    token: string,
    cohortDuration?: number,
    cfg?: unknown,
  ) => Record<string, unknown>;
  validateFixtureTokenAtInit: (
    token: string,
    cfg?: unknown,
    fixtureUserId?: string,
  ) => Record<string, unknown>;
  getAuthHeaders: (
    tokenOverride?: string,
    validateOptions?: unknown,
  ) => Record<string, string>;
  getPublicHeaders: () => Record<string, string>;
}

function createAuthModule(configMock: Record<string, unknown> = {}): AuthModule {
  const sandbox: Record<string, unknown> = {
    getConfig: () => ({
      baseUrl: "http://alt-backend:9000",
      apiToken: "",
      testUserId: "",
      testTenantId: "",
      ...configMock,
    }),
    exports: {},
    Buffer,
    atob,
    btoa,
    console,
    Date,
    Math,
    JSON,
    Array,
    String,
    Number,
    isNaN,
    Error,
  };
  vm.createContext(sandbox);
  vm.runInContext(jwtSource, sandbox);
  vm.runInContext(authSource, sandbox);
  return sandbox.exports as AuthModule;
}

const { validateFixtureTokenAtInit } = createAuthModule();

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

/** Build a minimal unsigned JWT with the given claims. */
function makeJWT(claims: Record<string, unknown>): string {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  const payload = btoa(JSON.stringify(claims))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return `${header}.${payload}.fake_sig`;
}

/** Returns a canonical set of valid claims anchored to `now`, with custom exp/iat. */
function validClaims(
  iatOffset: number,
  expOffset: number,
): Record<string, unknown> {
  const now = Math.floor(Date.now() / 1000);
  return {
    iss: "auth-hub",
    aud: "alt-backend",
    sub: "00000000-0000-0000-0000-000000000001",
    iat: now + iatOffset,
    exp: now + expOffset,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("validateFixtureTokenAtInit (via actual auth.js VM module loader)", () => {
  it("throws 'Token expired' for expired token (exp = now - 1)", () => {
    // exp in the past -> should fail the expiry check before the budget check
    // iat = now - 10, exp = now - 1  (window=9s, valid window, but already expired)
    const token = makeJWT(validClaims(-10, -1));
    assertThrows(
      () => validateFixtureTokenAtInit(token),
      Error,
      "Token expired",
    );
  });

  it("throws insufficient remaining for token with only 100s left (< 180s required)", () => {
    // iat = now - 10, exp = now + 100 -> window=110s (<=300s OK), remaining=100s < 180s
    const token = makeJWT(validClaims(-10, 100));
    assertThrows(
      () => validateFixtureTokenAtInit(token),
      Error,
      "insufficient",
    );
  });

  it("validates successfully when token has 200s remaining (>= 180s required)", () => {
    // iat = now - 10, exp = now + 200 -> window=210s (<=300s OK), remaining=200s >= 180s
    const token = makeJWT(validClaims(-10, 200));
    const claims = validateFixtureTokenAtInit(token);
    assertEquals(claims["iss"], "auth-hub");
    assertEquals(claims["sub"], "00000000-0000-0000-0000-000000000001");
  });

  it("positive: valid token with 200s remaining passes all checks and returns claims", () => {
    // Same as above but verifies the returned claims object is fully populated
    const token = makeJWT(validClaims(-10, 200));
    const claims = validateFixtureTokenAtInit(token);
    assertEquals(typeof claims["iat"], "number");
    assertEquals(typeof claims["exp"], "number");
    assertEquals(claims["aud"], "alt-backend");
    // remaining should be approximately 200s (allow +-2s for test execution time)
    const now = Math.floor(Date.now() / 1000);
    const remaining = (claims["exp"] as number) - now;
    assertEquals(remaining >= 198 && remaining <= 200, true);
  });

  it("throws on sub mismatch when fixture user_id differs from token sub", () => {
    const token = makeJWT(validClaims(-10, 200));
    assertThrows(
      () => validateFixtureTokenAtInit(token, {}, "00000000-0000-0000-0000-000000000099"),
      Error,
      "mismatch",
    );
  });

  it("throws when token validity duration exceeds 5 minutes (> 300s)", () => {
    // iat = now - 10, exp = now + 350 -> 360s window (> 300s)
    const token = makeJWT(validClaims(-10, 350));
    assertThrows(
      () => validateFixtureTokenAtInit(token),
      Error,
      "exceeds maximum allowed 5 minutes",
    );
  });
});
