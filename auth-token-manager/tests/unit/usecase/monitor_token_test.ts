import { afterEach, beforeEach, describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { MonitorTokenUsecase } from "../../../src/usecase/monitor_token.ts";
import type {
  SecretData,
  SecretManager,
  TokenResponse,
} from "../../../src/domain/types.ts";

function createMockSecretManager(
  tokenData: SecretData | null = null,
): SecretManager {
  return {
    getTokenSecret: () => Promise.resolve(tokenData),
    updateTokenSecret: (_tokens: TokenResponse) => Promise.resolve(),
    checkSecretExists: () => Promise.resolve(tokenData !== null),
  };
}

describe("MonitorTokenUsecase", {
  sanitizeResources: false,
  sanitizeOps: false,
}, () => {
  let originalEnv: Record<string, string | undefined>;

  beforeEach(() => {
    originalEnv = {
      INOREADER_CLIENT_ID: Deno.env.get("INOREADER_CLIENT_ID"),
      INOREADER_CLIENT_SECRET: Deno.env.get("INOREADER_CLIENT_SECRET"),
    };
    Deno.env.set("INOREADER_CLIENT_ID", "test-client-id-12345");
    Deno.env.set("INOREADER_CLIENT_SECRET", "test-client-secret-12345");
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

  it("should return info when token is healthy", async () => {
    const tokenData: SecretData = {
      access_token: "valid-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
      updated_at: new Date().toISOString(),
    };

    const secretManager = createMockSecretManager(tokenData);
    const usecase = new MonitorTokenUsecase(secretManager);
    const result = await usecase.execute();

    assertEquals(result.alertLevel, "info");
  });

  it("should return critical when token is expired", async () => {
    const tokenData: SecretData = {
      access_token: "expired-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() - 3600 * 1000).toISOString(), // 1 hour ago
      updated_at: new Date().toISOString(),
    };

    const secretManager = createMockSecretManager(tokenData);
    const usecase = new MonitorTokenUsecase(secretManager);
    const result = await usecase.execute();

    assertEquals(result.alertLevel, "critical");
    assertEquals(
      result.monitoringData.alerts.some((a) => a.includes("already expired")),
      true,
    );
  });

  it("should return warning when token expires within 2 hours", async () => {
    const tokenData: SecretData = {
      access_token: "expiring-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 90 * 60 * 1000).toISOString(), // 90 min
      updated_at: new Date().toISOString(),
    };

    const secretManager = createMockSecretManager(tokenData);
    const usecase = new MonitorTokenUsecase(secretManager);
    const result = await usecase.execute();

    assertEquals(result.alertLevel, "warning");
  });

  it("should return critical when no token data exists", async () => {
    const secretManager = createMockSecretManager(null);
    const usecase = new MonitorTokenUsecase(secretManager);
    const result = await usecase.execute();

    assertEquals(result.alertLevel, "critical");
  });

  it("should return critical when refresh token is invalid", async () => {
    const tokenData: SecretData = {
      access_token: "valid-access-token-1234567890",
      refresh_token: "short", // invalid - too short
      expires_at: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
      updated_at: new Date().toISOString(),
    };

    const secretManager = createMockSecretManager(tokenData);
    const usecase = new MonitorTokenUsecase(secretManager);
    const result = await usecase.execute();

    assertEquals(result.alertLevel, "critical");
    assertEquals(
      result.monitoringData.alerts.some((a) =>
        a.includes("Invalid or missing refresh token")
      ),
      true,
    );
  });

  it("should warn when token hasn't been updated for 12+ hours", async () => {
    const tokenData: SecretData = {
      access_token: "valid-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
      updated_at: new Date(
        Date.now() - 13 * 3600 * 1000,
      ).toISOString(), // 13 hours ago
    };

    const secretManager = createMockSecretManager(tokenData);
    const usecase = new MonitorTokenUsecase(secretManager);
    const result = await usecase.execute();

    assertEquals(result.alertLevel, "warning");
    assertEquals(
      result.monitoringData.alerts.some((a) => a.includes("12 hours")),
      true,
    );
  });
});
