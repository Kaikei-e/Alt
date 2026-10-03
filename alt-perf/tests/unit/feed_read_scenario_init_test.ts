/**
 * feed_read_scenario_init_test.ts
 *
 * Mocked VM / loader test for actual scenario init execution in feed-read-3000vu.js.
 * Verifies that the actual scenario file wires validateFixtureTokenAtInit for every
 * fixture in the SharedArray at init time, enforcing:
 *   - Remaining validity >= 180s
 *   - Token validity duration <= 5 minutes (300s)
 *   - Fixture user_id matches JWT sub claim (owner verification)
 *   - Fails closed at init time if short/expired/mismatched.
 */

import { assertEquals, assertThrows } from "@std/assert";
import { describe, it } from "@std/testing/bdd";
import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";
import { Buffer } from "node:buffer";

const UUID_REGEX =
  /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const MAX_JWT_LENGTH = 4096;
const MAX_TTL_SECONDS = 300;
const CLOCK_SKEW_SECONDS = 60;

function decodeJWT(token: string): Record<string, unknown> | null {
  try {
    if (typeof token !== "string") return null;
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    let base64 = parts[1]!.replace(/-/g, "+").replace(/_/g, "/");
    while (base64.length % 4) base64 += "=";
    const payloadStr = atob(base64);
    return JSON.parse(payloadStr) as Record<string, unknown>;
  } catch {
    return null;
  }
}

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

function validClaims(
  userId: string,
  iatOffset: number,
  expOffset: number,
): Record<string, unknown> {
  const now = Math.floor(Date.now() / 1000);
  return {
    iss: "auth-hub",
    aud: "alt-backend",
    sub: userId,
    iat: now + iatOffset,
    exp: now + expOffset,
  };
}


function runScenarioInit(
  fixtures: Record<string, unknown>[],
  envOverrides: Record<string, string> = {},
): { validatedCount: number } {
  const scenarioPath = fileURLToPath(
    new URL("../../k6/scenarios/feed-read-3000vu.js", import.meta.url),
  );
  let code = fs.readFileSync(scenarioPath, "utf-8");

  // Transform ES module imports and exports for VM execution
  code = code.replace(
    /import\s+\*\s+as\s+(\w+)\s+from\s+["\x27][^"\x27]+["\x27];?/g,
    "const $1 = __mocks__['$1'];",
  );
  code = code.replace(
    /import\s+([^{}\s,]+)\s+from\s+["\x27][^"\x27]+["\x27];?/g,
    "const $1 = __mocks__['$1'];",
  );
  code = code.replace(
    /import\s*\{([^}]+)\}\s*from\s*["\x27][^"\x27]+["\x27];?/g,
    "const {$1} = __mocks__;",
  );
  code = code.replace(/export\s+default\s+function\s*\(/g, "function default_func(");
  code = code.replace(/export\s+/g, "");

  const jwtPath = fileURLToPath(new URL("../../k6/helpers/jwt.js", import.meta.url));
  const authPath = fileURLToPath(new URL("../../k6/helpers/auth.js", import.meta.url));
  const jwtSource = fs.readFileSync(jwtPath, "utf-8").replace(/export\s+/g, "");
  const authRaw = fs
    .readFileSync(authPath, "utf-8")
    .replace(/import\s*\{[^}]*getConfig[^}]*\}\s*from\s*["\x27][^"\x27]+["\x27];?/g, "")
    .replace(/import\s*\{[^}]*decodeJWT[^}]*\}\s*from\s*["\x27][^"\x27]+["\x27];?/g, "")
    .replace(/export\s+function\s+(\w+)/g, "function $1");
  const authSource = `${jwtSource}\n${authRaw}\nObject.assign(exports, { validateAuthHubToken, validateCohortTokenAtInit, validateFixtureTokenAtInit, getAuthHeaders, getPublicHeaders });`;

  const exportsObj: Record<string, unknown> = {};
  const configFn = () => ({
    baseUrl: "http://alt-backend:9000",
    apiToken: envOverrides.K6_API_TOKEN || "",
    testUserId: envOverrides.K6_TEST_USER_ID || "",
    testTenantId: envOverrides.K6_TEST_TENANT_ID || "",
  });
  const authSandbox = { exports: exportsObj, getConfig: configFn, console, Buffer };
  vm.createContext(authSandbox);
  vm.runInContext(authSource, authSandbox);

  let validatedCount = 0;

  const sandbox: Record<string, unknown> = {
    __ENV: {
      USERS_FILE: "/mock/users.sample.json",
      ...envOverrides,
    },
    open: (_filePath: string) => JSON.stringify(fixtures),
    __mocks__: {
      http: {
        post: () => ({ status: 200, timings: { duration: 10 } }),
      },
      check: () => true,
      sleep: () => {},
      SharedArray: class {
        name: string;
        data: unknown[];
        constructor(name: string, fn: () => unknown[]) {
          this.name = name;
          this.data = fn(); // executes callback immediately during init
        }
        get length() {
          return this.data.length;
        }
      },
      Counter: class {
        add() {}
      },
      Trend: class {
        add() {}
      },
      getConfig: configFn,
      validateCohortTokenAtInit: exportsObj.validateCohortTokenAtInit,
      validateFixtureTokenAtInit: (
        token: string,
        cfg: { testUserId?: string } = {},
        fixtureUserId?: string,
      ) => {
        const claims = (exportsObj.validateFixtureTokenAtInit as Function)(token, cfg, fixtureUserId);
        validatedCount++;
        return claims;
      },
      getAuthHeaders: exportsObj.getAuthHeaders,
      handleSummary: () => ({}),
    },
    console,
  };
  vm.createContext(sandbox);
  vm.runInContext(code, sandbox);

  return { validatedCount };
}

describe("feed-read-3000vu.js scenario init fixture validation", () => {
  const user1 = "00000000-0000-4000-a000-000000000001";
  const user2 = "00000000-0000-4000-a000-000000000002";

  it("passes scenario init when all fixture tokens have >= 180s remaining and matching user_id", () => {
    const token1 = makeJWT(validClaims(user1, -10, 200));
    const token2 = makeJWT(validClaims(user2, -10, 250));

    const fixtures = [
      { user_id: user1, token: token1 },
      { user_id: user2, token: token2 },
    ];

    const result = runScenarioInit(fixtures);
    assertEquals(result.validatedCount, 2);
  });

  it("fails scenario init if a fixture token is expired", () => {
    const expiredToken = makeJWT(validClaims(user1, -10, -1));
    const fixtures = [{ user_id: user1, token: expiredToken }];

    try {
      runScenarioInit(fixtures);
      throw new Error("Expected to throw");
    } catch (e: any) {
      if (!e.message || !e.message.includes("Token expired")) {
        throw new Error(`Expected error message to include "Token expired", got ${e.message}`);
      }
    }
  });

  it("fails scenario init if a fixture token has insufficient remaining validity (< 180s)", () => {
    // 100s remaining < 180s required
    const shortToken = makeJWT(validClaims(user1, -10, 100));
    const fixtures = [{ user_id: user1, token: shortToken }];

    try {
      runScenarioInit(fixtures);
      throw new Error("Expected to throw");
    } catch (e: any) {
      if (!e.message || !e.message.includes("insufficient")) {
        throw new Error(`Expected error message to include "insufficient", got ${e.message}`);
      }
    }
  });

  it("fails scenario init if a fixture token validity exceeds 5 minutes (> 300s)", () => {
    // 350s window > 300s max TTL
    const longToken = makeJWT(validClaims(user1, -10, 350));
    const fixtures = [{ user_id: user1, token: longToken }];

    try {
      runScenarioInit(fixtures);
      throw new Error("Expected to throw");
    } catch (e: any) {
      if (!e.message || !e.message.includes("exceeds maximum allowed 5 minutes")) {
        throw new Error(`Expected error message to include "exceeds maximum allowed 5 minutes", got ${e.message}`);
      }
    }
  });

  it("fails scenario init if fixture user_id mismatches token sub (owner verification)", () => {
    // token sub is user1, but fixture user_id is user2
    const token = makeJWT(validClaims(user1, -10, 200));
    const fixtures = [{ user_id: user2, token: token }];

    try {
      runScenarioInit(fixtures);
      throw new Error("Expected to throw");
    } catch (e: any) {
      if (!e.message || !e.message.includes("mismatch")) {
        throw new Error(`Expected error message to include "mismatch", got ${e.message}`);
      }
    }
  });
});
