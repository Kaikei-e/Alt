/**
 * Retry policy unit tests
 * TDD: These tests define the expected behavior of the retry mechanism
 */
import { assertEquals, assertRejects } from "@std/assert";
import { describe, it, beforeEach } from "@std/testing/bdd";
import { spy, stub, restore } from "@std/testing/mock";
import {
  RetryExecutor,
  DEFAULT_RETRY_POLICY,
  type RetryPolicy,
  type RetryResult,
} from "../../src/retry/retry-policy.ts";

describe("RetryExecutor", () => {
  describe("execute", () => {
    it("should succeed on first attempt without retry", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      const operation = () => Promise.resolve("success");

      const result = await executor.execute(operation);

      assertEquals(result.success, true);
      assertEquals(result.value, "success");
      assertEquals(result.attempts, 1);
      assertEquals(result.errors.length, 0);
    });

    it("should retry on transient failure and succeed", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts < 3) {
          const error = new Error("temporary failure");
          error.name = "TimeoutError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const result = await executor.execute(operation);

      assertEquals(result.success, true);
      assertEquals(result.value, "success");
      assertEquals(result.attempts, 3);
      assertEquals(result.errors.length, 2);
    });

    it("should not retry non-retryable errors", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      const operation = () => {
        const error = new Error("invalid input");
        error.name = "ValidationError";
        throw error;
      };

      const result = await executor.execute(operation);

      assertEquals(result.success, false);
      assertEquals(result.attempts, 1);
      assertEquals(result.errors.length, 1);
    });

    it("should call onRetry callback on each retry", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      const onRetryCalls: number[] = [];
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts < 3) {
          const error = new Error("failure");
          error.name = "TimeoutError";
          throw error;
        }
        return Promise.resolve("success");
      };

      await executor.execute(operation, (context) => {
        onRetryCalls.push(context.attempt);
      });

      assertEquals(onRetryCalls.length, 2);
      assertEquals(onRetryCalls[0], 2);
      assertEquals(onRetryCalls[1], 3);
    });

    it("should respect maxAttempts limit", async () => {
      const policy: RetryPolicy = { ...DEFAULT_RETRY_POLICY, maxAttempts: 2 };
      const executor = new RetryExecutor(policy);
      let attempts = 0;
      const operation = () => {
        attempts++;
        const error = new Error("always fails");
        error.name = "TimeoutError";
        throw error;
      };

      const result = await executor.execute(operation);

      assertEquals(result.success, false);
      assertEquals(result.attempts, 2);
      assertEquals(attempts, 2);
    });

    it("should apply exponential backoff delay", async () => {
      const policy: RetryPolicy = {
        ...DEFAULT_RETRY_POLICY,
        baseDelayMs: 50,
        maxDelayMs: 500,
        backoffMultiplier: 2,
        maxAttempts: 3,
      };
      const executor = new RetryExecutor(policy);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts < 3) {
          const error = new Error("failure");
          error.name = "TimeoutError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const startTime = performance.now();
      await executor.execute(operation);
      const duration = performance.now() - startTime;

      // First retry: 50ms, second retry: 100ms = 150ms minimum (with jitter)
      assertEquals(duration >= 100, true);
    });

    it("should cap delay at maxDelayMs", async () => {
      const policy: RetryPolicy = {
        ...DEFAULT_RETRY_POLICY,
        baseDelayMs: 100,
        maxDelayMs: 150,
        backoffMultiplier: 10,
        maxAttempts: 3,
      };
      const executor = new RetryExecutor(policy);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts < 3) {
          const error = new Error("failure");
          error.name = "TimeoutError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const startTime = performance.now();
      await executor.execute(operation);
      const duration = performance.now() - startTime;

      // Should not exceed 150ms * 2 retries = 300ms significantly
      assertEquals(duration < 400, true);
    });

    it("should track total duration", async () => {
      const policy: RetryPolicy = {
        ...DEFAULT_RETRY_POLICY,
        baseDelayMs: 50,
        maxAttempts: 2,
      };
      const executor = new RetryExecutor(policy);
      const operation = () => {
        const error = new Error("failure");
        error.name = "TimeoutError";
        throw error;
      };

      const result = await executor.execute(operation);

      assertEquals(result.totalDuration > 0, true);
      assertEquals(result.totalDuration >= 50, true);
    });

    it("should handle async operation errors", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      let attempts = 0;
      const operation = async () => {
        attempts++;
        await new Promise((resolve) => setTimeout(resolve, 10));
        if (attempts < 2) {
          const error = new Error("async failure");
          error.name = "NetworkError";
          throw error;
        }
        return "success";
      };

      const result = await executor.execute(operation);

      assertEquals(result.success, true);
      assertEquals(result.attempts, 2);
    });
  });

  describe("isRetryable", () => {
    it("should identify TimeoutError as retryable", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts === 1) {
          const error = new Error("timeout");
          error.name = "TimeoutError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const result = await executor.execute(operation);
      assertEquals(result.success, true);
      assertEquals(result.attempts, 2);
    });

    it("should identify NavigationError as retryable", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts === 1) {
          const error = new Error("navigation failed");
          error.name = "NavigationError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const result = await executor.execute(operation);
      assertEquals(result.success, true);
    });

    it("should identify NetworkError as retryable", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts === 1) {
          const error = new Error("network error");
          error.name = "NetworkError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const result = await executor.execute(operation);
      assertEquals(result.success, true);
    });

    it("should identify ProtocolError as retryable", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts === 1) {
          const error = new Error("protocol error");
          error.name = "ProtocolError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const result = await executor.execute(operation);
      assertEquals(result.success, true);
    });

    it("should not retry unknown error types", async () => {
      const executor = new RetryExecutor(DEFAULT_RETRY_POLICY);
      const operation = () => {
        const error = new Error("unknown");
        error.name = "UnknownError";
        throw error;
      };

      const result = await executor.execute(operation);
      assertEquals(result.success, false);
      assertEquals(result.attempts, 1);
    });
  });

  describe("custom retry policy", () => {
    it("should allow custom retryable errors", async () => {
      const policy: RetryPolicy = {
        ...DEFAULT_RETRY_POLICY,
        retryableErrors: ["CustomError"],
      };
      const executor = new RetryExecutor(policy);
      let attempts = 0;
      const operation = () => {
        attempts++;
        if (attempts === 1) {
          const error = new Error("custom");
          error.name = "CustomError";
          throw error;
        }
        return Promise.resolve("success");
      };

      const result = await executor.execute(operation);
      assertEquals(result.success, true);
      assertEquals(result.attempts, 2);
    });
  });
});

describe("DEFAULT_RETRY_POLICY", () => {
  it("should have expected default values", () => {
    assertEquals(DEFAULT_RETRY_POLICY.maxAttempts, 3);
    assertEquals(DEFAULT_RETRY_POLICY.baseDelayMs, 1000);
    assertEquals(DEFAULT_RETRY_POLICY.maxDelayMs, 10000);
    assertEquals(DEFAULT_RETRY_POLICY.backoffMultiplier, 2);
    assertEquals(DEFAULT_RETRY_POLICY.retryableErrors.includes("TimeoutError"), true);
    assertEquals(DEFAULT_RETRY_POLICY.retryableErrors.includes("NavigationError"), true);
    assertEquals(DEFAULT_RETRY_POLICY.retryableErrors.includes("NetworkError"), true);
    assertEquals(DEFAULT_RETRY_POLICY.retryableErrors.includes("ProtocolError"), true);
  });
});
