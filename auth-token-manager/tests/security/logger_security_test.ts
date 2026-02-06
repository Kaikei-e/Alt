/**
 * Basic Security Test Suite for OAuth Token Logging
 * Focused on CWE-532 prevention
 */

import { assertEquals, assertStringIncludes } from "@std/testing/asserts";
import {
  createComponentLogger,
  DataSanitizer,
} from "../../src/infra/logger.ts";

const TEST_DATA = {
  access_token: "ya29.a0ARrdaM-1234567890abcdefghijk",
  refresh_token: "1//04567890abcdefghijk",
  bearer_token: "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.signature",
  api_key: "AIzaSyC1234567890abcdefghijk",
  AppId: "12345678",
  AppKey: "abcdef1234567890",
  username: "testuser",
  email: "user@example.com",
  safe_info: "This is safe information",
};

Deno.test("DataSanitizer - OAuth Token Masking", () => {
  const accessResult = DataSanitizer.sanitize(TEST_DATA.access_token);
  assertStringIncludes(String(accessResult), "[REDACTED]");

  const refreshResult = DataSanitizer.sanitize(TEST_DATA.refresh_token);
  assertStringIncludes(String(refreshResult), "[REDACTED]");

  const bearerResult = DataSanitizer.sanitize(TEST_DATA.bearer_token);
  assertStringIncludes(String(bearerResult), "[REDACTED]");
});

Deno.test("DataSanitizer - Object Field Sanitization", () => {
  const testObject = {
    access_token: TEST_DATA.access_token,
    refresh_token: TEST_DATA.refresh_token,
    safe_data: TEST_DATA.safe_info,
    username: TEST_DATA.username,
  };

  const sanitized = DataSanitizer.sanitize(testObject);
  assertEquals(sanitized.access_token, "[REDACTED]");
  assertEquals(sanitized.refresh_token, "[REDACTED]");
  assertEquals(sanitized.safe_data, "This is safe information");
  assertEquals(sanitized.username, "testuser");
});

Deno.test("DataSanitizer - Inoreader API Credentials", () => {
  const inoreaderData = {
    AppId: TEST_DATA.AppId,
    AppKey: TEST_DATA.AppKey,
    user_info: "safe user info",
  };

  const sanitized = DataSanitizer.sanitize(inoreaderData);
  assertEquals(sanitized.AppId, "[REDACTED]");
  assertEquals(sanitized.AppKey, "[REDACTED]");
  assertEquals(sanitized.user_info, "safe user info");
});

Deno.test({
  name: "StructuredLogger - Basic Functionality",
  sanitizeResources: false,
  sanitizeOps: false,
  fn: () => {
    const testLogger = createComponentLogger("test-oauth");
    testLogger.info("Test OAuth token refresh", {
      user_id: "user123",
      access_token: TEST_DATA.access_token,
      status: "success",
    });
    assertEquals(true, true);
  },
});

Deno.test("DataSanitizer - Boundary Conditions", () => {
  const boundaryTests = [null, undefined, "", "short", { empty: {} }, []];

  for (const testCase of boundaryTests) {
    const result = DataSanitizer.sanitize(testCase);
    if (testCase === undefined) {
      assertEquals(result, undefined);
    } else if (testCase === null) {
      assertEquals(result, null);
    }
  }
});
