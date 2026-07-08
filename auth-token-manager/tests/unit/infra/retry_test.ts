import { describe, it } from "@std/testing/bdd";
import { assertEquals, assertRejects } from "@std/testing/asserts";
import { PermanentError, retryWithBackoff } from "../../../src/infra/retry.ts";
import type { RetryConfig } from "../../../src/domain/types.ts";

const fastRetryConfig: RetryConfig = {
  max_attempts: 3,
  base_delay: 1,
  max_delay: 5,
  backoff_factor: 2,
};

describe(
  "retryWithBackoff",
  { sanitizeResources: false, sanitizeOps: false },
  () => {
    it("returns the result on first success without retrying", async () => {
      let calls = 0;
      const result = await retryWithBackoff(() => {
        calls++;
        return Promise.resolve("ok");
      }, fastRetryConfig);

      assertEquals(result, "ok");
      assertEquals(calls, 1);
    });

    it("retries transient errors up to max_attempts", async () => {
      let calls = 0;
      await assertRejects(
        () =>
          retryWithBackoff(() => {
            calls++;
            return Promise.reject(new Error("transient failure"));
          }, fastRetryConfig),
        Error,
        "transient failure",
      );

      assertEquals(calls, fastRetryConfig.max_attempts);
    });

    it("succeeds after a transient failure on a later attempt", async () => {
      let calls = 0;
      const result = await retryWithBackoff(() => {
        calls++;
        if (calls < 2) {
          return Promise.reject(new Error("temporary"));
        }
        return Promise.resolve("recovered");
      }, fastRetryConfig);

      assertEquals(result, "recovered");
      assertEquals(calls, 2);
    });

    it("fails fast on PermanentError without retrying", async () => {
      let calls = 0;
      await assertRejects(
        () =>
          retryWithBackoff(() => {
            calls++;
            return Promise.reject(new PermanentError("invalid_grant"));
          }, fastRetryConfig),
        PermanentError,
        "invalid_grant",
      );

      assertEquals(calls, 1);
    });
  },
);
