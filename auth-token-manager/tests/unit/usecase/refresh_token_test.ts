import { describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { RefreshTokenUsecase } from "../../../src/usecase/refresh_token.ts";
import type {
  NetworkConfig,
  RetryConfig,
  SecretData,
  SecretManager,
  TokenResponse,
} from "../../../src/domain/types.ts";
import type { TokenClient } from "../../../src/port/token_client.ts";
import type { HttpClient } from "../../../src/port/http_client.ts";

function createMockSecretManager(
  tokenData: SecretData | null = null,
): SecretManager {
  let stored: SecretData | null = tokenData;
  return {
    getTokenSecret: () => Promise.resolve(stored),
    updateTokenSecret: (tokens: TokenResponse) => {
      stored = {
        access_token: tokens.access_token,
        refresh_token: tokens.refresh_token,
        expires_at: tokens.expires_at.toISOString(),
        updated_at: new Date().toISOString(),
        token_type: tokens.token_type,
        scope: tokens.scope,
      };
      return Promise.resolve();
    },
    checkSecretExists: () => Promise.resolve(stored !== null),
  };
}

function createMockTokenClient(
  response?: TokenResponse,
  shouldFail = false,
): TokenClient {
  return {
    refreshToken: () => {
      if (shouldFail) {
        return Promise.reject(new Error("Token refresh API error"));
      }
      return Promise.resolve(
        response || {
          access_token: "new-access-token-1234567890",
          refresh_token: "new-refresh-token-1234567890",
          expires_at: new Date(Date.now() + 3600 * 1000),
          token_type: "Bearer",
          scope: "read write",
        },
      );
    },
    exchangeCode: () => Promise.reject(new Error("Not used in refresh")),
  };
}

function createMockHttpClient(): HttpClient {
  return {
    fetch: () => Promise.resolve(new Response("", { status: 200 })),
  };
}

const defaultRetryConfig: RetryConfig = {
  max_attempts: 1,
  base_delay: 10,
  max_delay: 100,
  backoff_factor: 2,
};

const defaultNetworkConfig: NetworkConfig = {
  http_timeout: 5000,
  connectivity_check: false,
  connectivity_timeout: 5000,
};

describe("RefreshTokenUsecase", {
  sanitizeResources: false,
  sanitizeOps: false,
}, () => {
  it("should refresh token successfully when token is near expiry", async () => {
    const expiredTokenData: SecretData = {
      access_token: "old-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 60 * 1000).toISOString(), // 1 min left
      updated_at: new Date().toISOString(),
      token_type: "Bearer",
      scope: "read write",
    };

    const secretManager = createMockSecretManager(expiredTokenData);
    const tokenClient = createMockTokenClient();
    const httpClient = createMockHttpClient();

    const usecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      defaultNetworkConfig,
      defaultRetryConfig,
    );

    const result = await usecase.execute();

    assertEquals(result.success, true);
    assertEquals(result.tokens?.access_token, "new-access-token-1234567890");
    assertEquals(result.metadata?.method, "refresh_token");
  });

  it("should skip refresh when token is still valid", async () => {
    const validTokenData: SecretData = {
      access_token: "still-valid-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000).toISOString(), // 1 hour left
      updated_at: new Date().toISOString(),
      token_type: "Custom",
      scope: "read",
    };

    const secretManager = createMockSecretManager(validTokenData);
    const tokenClient = createMockTokenClient();
    const httpClient = createMockHttpClient();

    const usecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      defaultNetworkConfig,
      defaultRetryConfig,
    );

    const result = await usecase.execute();

    assertEquals(result.success, true);
    assertEquals(
      result.tokens?.access_token,
      "still-valid-access-token-1234567890",
    );
    assertEquals(result.tokens?.token_type, "Custom");
    assertEquals(result.tokens?.scope, "read");
  });

  it("should fail gracefully when no refresh token exists", async () => {
    const secretManager = createMockSecretManager(null);
    const tokenClient = createMockTokenClient();
    const httpClient = createMockHttpClient();

    const usecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      defaultNetworkConfig,
      defaultRetryConfig,
    );

    const result = await usecase.execute();

    assertEquals(result.success, false);
    assertEquals(
      result.error?.includes("No existing refresh token"),
      true,
    );
  });

  it("should fail gracefully when token client errors", async () => {
    const tokenData: SecretData = {
      access_token: "old-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 60 * 1000).toISOString(),
      updated_at: new Date().toISOString(),
    };

    const secretManager = createMockSecretManager(tokenData);
    const tokenClient = createMockTokenClient(undefined, true);
    const httpClient = createMockHttpClient();

    const usecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      defaultNetworkConfig,
      defaultRetryConfig,
    );

    const result = await usecase.execute();

    assertEquals(result.success, false);
    assertEquals(result.error?.includes("Token refresh API error"), true);
  });

  it("should reject invalid refresh token format", async () => {
    const invalidTokenData: SecretData = {
      access_token: "old-access-token-1234567890",
      refresh_token: "short", // too short
      expires_at: new Date(Date.now() + 60 * 1000).toISOString(),
      updated_at: new Date().toISOString(),
    };

    const secretManager = createMockSecretManager(invalidTokenData);
    const tokenClient = createMockTokenClient();
    const httpClient = createMockHttpClient();

    const usecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      defaultNetworkConfig,
      defaultRetryConfig,
    );

    const result = await usecase.execute();

    assertEquals(result.success, false);
    assertEquals(result.error?.includes("Invalid refresh token format"), true);
  });

  it("should retry only the write (not the non-idempotent token refresh) when persisting fails", async () => {
    const nearExpiryTokenData: SecretData = {
      access_token: "old-access-token-1234567890",
      refresh_token: "valid-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 60 * 1000).toISOString(), // 1 min left
      updated_at: new Date().toISOString(),
      token_type: "Bearer",
      scope: "read write",
    };

    const rotatedTokens: TokenResponse = {
      access_token: "rotated-access-token-1234567890",
      refresh_token: "rotated-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
      token_type: "Bearer",
      scope: "read write",
    };

    let refreshCallCount = 0;
    const tokenClient: TokenClient = {
      refreshToken: () => {
        refreshCallCount++;
        return Promise.resolve(rotatedTokens);
      },
      exchangeCode: () => Promise.reject(new Error("Not used in refresh")),
    };

    let updateCallCount = 0;
    let persistedTokens: TokenResponse | null = null;
    const secretManager: SecretManager = {
      getTokenSecret: () => Promise.resolve(nearExpiryTokenData),
      updateTokenSecret: (tokens: TokenResponse) => {
        updateCallCount++;
        if (updateCallCount === 1) {
          return Promise.reject(new Error("disk write failed"));
        }
        persistedTokens = tokens;
        return Promise.resolve();
      },
      checkSecretExists: () => Promise.resolve(true),
    };

    const httpClient = createMockHttpClient();
    const retryConfig: RetryConfig = {
      max_attempts: 2,
      base_delay: 1,
      max_delay: 10,
      backoff_factor: 2,
    };

    const usecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      defaultNetworkConfig,
      retryConfig,
    );

    const result = await usecase.execute();

    assertEquals(result.success, true);
    // The refresh call rotates the refresh_token server-side (non-idempotent).
    // It must never be retried, or a retry would resend the already-consumed
    // refresh_token and fail with invalid_grant.
    assertEquals(refreshCallCount, 1);
    // Only the (idempotent) persistence step is retried.
    assertEquals(updateCallCount, 2);
    assertEquals(
      persistedTokens!.refresh_token,
      "rotated-refresh-token-1234567890",
    );
    assertEquals(
      result.tokens?.refresh_token,
      "rotated-refresh-token-1234567890",
    );
  });
});
