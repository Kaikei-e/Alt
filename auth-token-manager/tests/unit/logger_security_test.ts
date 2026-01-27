/**
 * Security Tests for Logger Module
 *
 * Tests to ensure sensitive data is not logged in plain text
 */

import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.224.0/assert/mod.ts";
import {
  createComponentLogger,
  DataSanitizer,
  logger,
  StructuredLogger,
} from "../../src/utils/logger.ts";

// Helper function for assertNotStringIncludes since it's not available
function assertNotStringIncludes(
  actual: string,
  expected: string,
  msg?: string,
) {
  if (actual.includes(expected)) {
    throw new Error(msg || `Expected "${actual}" not to contain "${expected}"`);
  }
}

Deno.test({
  name: "Logger Security - Should not log sensitive authentication data",
  fn: async () => {
    // Capture console output
    const originalConsoleDebug = console.debug;
    const originalConsoleInfo = console.info;
    const originalConsoleWarn = console.warn;
    const originalConsoleError = console.error;
    const originalConsoleLog = console.log;

    let loggedMessages: string[] = [];

    // Override console methods to capture output
    console.debug = (message: string) => {
      loggedMessages.push(message);
    };
    console.info = (message: string) => {
      loggedMessages.push(message);
    };
    console.warn = (message: string) => {
      loggedMessages.push(message);
    };
    console.error = (message: string) => {
      loggedMessages.push(message);
    };
    console.log = (message: string) => {
      loggedMessages.push(message);
    };

    try {
      const testLogger = new StructuredLogger("test-logger");

      // Test cases with sensitive data
      const sensitiveData = {
        token:
          "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
        access_token: "ya29.a0AfH6SMA_-_-_SENSITIVE_TOKEN_-_-_",
        refresh_token: "ya29.r0AfH6SMA_-_-_REFRESH_TOKEN_-_-_",
        password: "superSecretPassword123!",
        api_key: "AIzaSyD-_-_-_-_API_KEY_-_-_-_",
        client_secret: "GOCSPX-_-_-_CLIENT_SECRET_-_-_",
        authorization_code: "authorization_code_12345",
        session_id: "sess_1234567890abcdef",
        user_id: "user_sensitive_123",
        email: "sensitive@example.com",
        credit_card: "4111-1111-1111-1111",
        ssn: "123-45-6789",
      };

      // Test logging operations that might expose sensitive data
      testLogger.info("Authentication successful", sensitiveData);
      testLogger.debug("OAuth token received", { token: sensitiveData.token });
      testLogger.warn("Token refresh required", {
        refresh_token: sensitiveData.refresh_token,
      });
      testLogger.error("Authentication failed", {
        password: sensitiveData.password,
      });
      // Note: critical() is not implemented in StructuredLogger, use error() instead
      testLogger.error("API key validation failed", {
        api_key: sensitiveData.api_key,
      });

      // Test with mixed data (some sensitive, some not)
      testLogger.info("User login attempt", {
        username: "john.doe",
        password: sensitiveData.password,
        ip_address: "192.168.1.1",
        user_agent: "Mozilla/5.0...",
        timestamp: new Date().toISOString(),
      });

      // Verify that sensitive data is present in logs (this should fail after fix)
      let foundSensitiveData = false;
      for (const message of loggedMessages) {
        // Check if any sensitive values appear in logs
        if (
          message.includes(sensitiveData.token) ||
          message.includes(sensitiveData.access_token) ||
          message.includes(sensitiveData.refresh_token) ||
          message.includes(sensitiveData.password) ||
          message.includes(sensitiveData.api_key) ||
          message.includes(sensitiveData.client_secret) ||
          message.includes(sensitiveData.authorization_code) ||
          message.includes(sensitiveData.credit_card) ||
          message.includes(sensitiveData.ssn)
        ) {
          foundSensitiveData = true;
          break;
        }
      }

      // After implementing data sanitization, sensitive data should NOT be found
      assertEquals(
        foundSensitiveData,
        false,
        "Sensitive data should NOT be found in logs after sanitization fix",
      );
    } finally {
      // Restore original console methods
      console.debug = originalConsoleDebug;
      console.info = originalConsoleInfo;
      console.warn = originalConsoleWarn;
      console.error = originalConsoleError;
      console.log = originalConsoleLog;
    }
  },
});

Deno.test({
  name: "Logger Security - Should sanitize sensitive data correctly",
  fn: async () => {
    // Test direct sanitization using DataSanitizer (not StructuredLogger method)
    const sensitiveData = {
      token: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
      password: "secretPassword123",
      api_key: "AIzaSyD12345678901234567890",
      normal_field: "this should not be redacted",
    };

    const sanitized = DataSanitizer.sanitize(sensitiveData);

    // Verify sensitive fields are sanitized
    assertEquals(typeof sanitized.token, "string");
    assertStringIncludes(sanitized.token, "[REDACTED]");

    assertEquals(typeof sanitized.password, "string");
    assertStringIncludes(sanitized.password, "[REDACTED]");

    assertEquals(typeof sanitized.api_key, "string");
    assertStringIncludes(sanitized.api_key, "[REDACTED]");

    // Verify non-sensitive fields are preserved
    assertEquals(sanitized.normal_field, "this should not be redacted");
  },
});

Deno.test({
  name: "Logger Security - Should preserve non-sensitive data",
  fn: async () => {
    const originalConsoleInfo = console.info;
    let loggedMessage = "";

    console.info = (message: string) => {
      loggedMessage = message;
    };

    try {
      const testLogger = new StructuredLogger("preserve-test");

      const nonSensitiveData = {
        username: "john.doe",
        ip_address: "192.168.1.1",
        timestamp: new Date().toISOString(),
        operation: "login",
        success: true,
      };

      testLogger.info("User operation", nonSensitiveData);

      // Verify non-sensitive data is preserved
      assertStringIncludes(loggedMessage, "john.doe");
      assertStringIncludes(loggedMessage, "192.168.1.1");
      assertStringIncludes(loggedMessage, "login");
    } finally {
      console.info = originalConsoleInfo;
    }
  },
});

Deno.test({
  name: "Logger Security - Environment variables exposure test",
  fn: async () => {
    const originalConsoleInfo = console.info;
    let loggedMessage = "";

    console.info = (message: string) => {
      loggedMessage = message;
    };

    try {
      // Set some sensitive environment variables for testing
      Deno.env.set("TEST_SECRET_KEY", "very-secret-key-123");
      Deno.env.set("TEST_API_TOKEN", "secret-api-token-456");

      const testLogger = new StructuredLogger("env-test");

      // Log environment data (common mistake)
      const envData = {
        NODE_ENV: Deno.env.get("NODE_ENV"),
        SECRET_KEY: Deno.env.get("TEST_SECRET_KEY"),
        API_TOKEN: Deno.env.get("TEST_API_TOKEN"),
      };

      testLogger.info("Environment configuration", envData);

      // After sanitization, secret data should NOT be found
      const containsSecret = loggedMessage.includes("very-secret-key-123") ||
        loggedMessage.includes("secret-api-token-456");

      assertEquals(
        containsSecret,
        false,
        "Environment secrets should NOT be found in logs after sanitization",
      );
    } finally {
      console.info = originalConsoleInfo;
      // Cleanup
      Deno.env.delete("TEST_SECRET_KEY");
      Deno.env.delete("TEST_API_TOKEN");
    }
  },
});
