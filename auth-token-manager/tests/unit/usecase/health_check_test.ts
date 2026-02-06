import { afterEach, beforeEach, describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { HealthCheckUsecase } from "../../../src/usecase/health_check.ts";
import type {
  SecretData,
  SecretManager,
  TokenResponse,
} from "../../../src/domain/types.ts";

function createMockSecretManager(
  tokenData: SecretData | null = null,
  shouldFail = false,
): SecretManager {
  return {
    getTokenSecret: () => {
      if (shouldFail) return Promise.reject(new Error("Storage error"));
      return Promise.resolve(tokenData);
    },
    updateTokenSecret: (_tokens: TokenResponse) => Promise.resolve(),
    checkSecretExists: () => Promise.resolve(tokenData !== null),
  };
}

describe(
  "HealthCheckUsecase",
  { sanitizeResources: false, sanitizeOps: false },
  () => {
    let originalEnv: Record<string, string | undefined>;

    beforeEach(() => {
      originalEnv = {
        INOREADER_CLIENT_ID: Deno.env.get("INOREADER_CLIENT_ID"),
        INOREADER_CLIENT_SECRET: Deno.env.get("INOREADER_CLIENT_SECRET"),
      };
    });

    afterEach(() => {
      for (const [key, value] of Object.entries(originalEnv)) {
        if (value === undefined) {
          Deno.env.delete(key);
        } else {
          Deno.env.set(key, value);
        }
      }
    });

    it("should return healthy when all checks pass", async () => {
      Deno.env.set("INOREADER_CLIENT_ID", "test-client-id-12345");
      Deno.env.set("INOREADER_CLIENT_SECRET", "test-client-secret-12345");

      const tokenData: SecretData = {
        access_token: "valid-access-token-1234567890",
        refresh_token: "valid-refresh-token-1234567890",
        expires_at: new Date(Date.now() + 7200 * 1000).toISOString(),
        updated_at: new Date().toISOString(),
        token_type: "Bearer",
        scope: "read write",
      };

      const secretManager = createMockSecretManager(tokenData);
      const usecase = new HealthCheckUsecase(secretManager);
      const result = await usecase.execute();

      assertEquals(result.status, "healthy");
      assertEquals(result.passing, 5);
      assertEquals(result.total, 5);
    });

    it("should return unhealthy when no tokens exist", async () => {
      Deno.env.delete("INOREADER_CLIENT_ID");
      Deno.env.delete("INOREADER_CLIENT_SECRET");

      const secretManager = createMockSecretManager(null);
      const usecase = new HealthCheckUsecase(secretManager);
      const result = await usecase.execute();

      assertEquals(result.status, "unhealthy");
    });

    it("should return degraded when storage is accessible but token is expiring", async () => {
      Deno.env.set("INOREADER_CLIENT_ID", "test-client-id-12345");
      Deno.env.set("INOREADER_CLIENT_SECRET", "test-client-secret-12345");

      const tokenData: SecretData = {
        access_token: "valid-access-token-1234567890",
        refresh_token: "valid-refresh-token-1234567890",
        expires_at: new Date(Date.now() + 30 * 60 * 1000).toISOString(), // 30 min
        updated_at: new Date().toISOString(),
        token_type: "Bearer",
        scope: "read write",
      };

      const secretManager = createMockSecretManager(tokenData);
      const usecase = new HealthCheckUsecase(secretManager);
      const result = await usecase.execute();

      assertEquals(result.status, "degraded");
      assertEquals(result.checks.token_expiry_status, false);
    });

    it("should handle storage errors gracefully", async () => {
      Deno.env.set("INOREADER_CLIENT_ID", "test-client-id-12345");
      Deno.env.set("INOREADER_CLIENT_SECRET", "test-client-secret-12345");

      const secretManager = createMockSecretManager(null, true);
      const usecase = new HealthCheckUsecase(secretManager);
      const result = await usecase.execute();

      assertEquals(result.checks.storage_ready, false);
    });
  },
);
