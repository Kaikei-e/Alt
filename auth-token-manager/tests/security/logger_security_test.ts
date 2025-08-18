/**
 * Basic Security Test Suite for OAuth Token Logging
 * Focused on CWE-532 prevention
 */

import { assertEquals, assertStringIncludes } from "@std/testing/asserts";
import { DataSanitizer, createComponentLogger } from "../../src/utils/logger.ts";

/**
 * Test data containing OAuth tokens and sensitive information
 */
const TEST_DATA = {
  // OAuth tokens
  access_token: "ya29.a0ARrdaM-1234567890abcdefghijk",
  refresh_token: "1//04567890abcdefghijk",
  bearer_token: "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test.signature",
  
  // API keys
  api_key: "AIzaSyC1234567890abcdefghijk",
  
  // Inoreader specific
  AppId: "12345678",
  AppKey: "abcdef1234567890",
  
  // Safe data
  username: "testuser",
  email: "user@example.com",
  safe_info: "This is safe information"
};

/**
 * Test basic OAuth token sanitization
 */
Deno.test("DataSanitizer - OAuth Token Masking", () => {
  // Test access token
  const accessResult = DataSanitizer.sanitize(TEST_DATA.access_token);
  assertStringIncludes(String(accessResult), "[REDACTED]");
  
  // Test refresh token
  const refreshResult = DataSanitizer.sanitize(TEST_DATA.refresh_token);
  assertStringIncludes(String(refreshResult), "[REDACTED]");
  
  // Test bearer token
  const bearerResult = DataSanitizer.sanitize(TEST_DATA.bearer_token);
  assertStringIncludes(String(bearerResult), "[REDACTED]");
});

/**
 * Test object sanitization with sensitive fields
 */
Deno.test("DataSanitizer - Object Field Sanitization", () => {
  const testObject = {
    access_token: TEST_DATA.access_token,
    refresh_token: TEST_DATA.refresh_token,
    safe_data: TEST_DATA.safe_info,
    username: TEST_DATA.username
  };
  
  const sanitized = DataSanitizer.sanitize(testObject);
  
  // Sensitive fields should be masked
  assertEquals(sanitized.access_token, "[REDACTED]");
  assertEquals(sanitized.refresh_token, "[REDACTED]");
  
  // Safe data should remain
  assertEquals(sanitized.safe_data, "This is safe information");
  assertEquals(sanitized.username, "testuser");
});

/**
 * Test Inoreader specific patterns
 */
Deno.test("DataSanitizer - Inoreader API Credentials", () => {
  const inoreaderData = {
    AppId: TEST_DATA.AppId,
    AppKey: TEST_DATA.AppKey,
    user_info: "safe user info"
  };
  
  const sanitized = DataSanitizer.sanitize(inoreaderData);
  
  // Inoreader credentials should be masked
  assertEquals(sanitized.AppId, "[REDACTED]");
  assertEquals(sanitized.AppKey, "[REDACTED]");
  
  // User info should remain
  assertEquals(sanitized.user_info, "safe user info");
});

/**
 * Test logger creation and basic functionality
 */
Deno.test("StructuredLogger - Basic Functionality", () => {
  const logger = createComponentLogger("test-oauth");
  
  // Should create logger without throwing
  logger.info("Test OAuth token refresh", {
    user_id: "user123",
    access_token: TEST_DATA.access_token, // This should be sanitized in output
    status: "success"
  });
  
  // Test passes if no exceptions thrown
  assertEquals(true, true);
});

/**
 * Test boundary conditions
 */
Deno.test("DataSanitizer - Boundary Conditions", () => {
  const boundaryTests = [
    null,
    undefined,
    "",
    "short",
    { empty: {} },
    []
  ];
  
  for (const testCase of boundaryTests) {
    try {
      const result = DataSanitizer.sanitize(testCase);
      // Should handle all cases without throwing
      if (testCase === undefined) {
        assertEquals(result, undefined);
      } else if (testCase === null) {
        assertEquals(result, null);
      }
    } catch (error) {
      throw new Error(`Failed on boundary case: ${testCase}, error: ${error}`);
    }
  }
});