import { describe, it } from "@std/testing/bdd";
import {
  assertEquals,
  assertRejects,
  assertThrows,
} from "@std/testing/asserts";
import { GetTokenUsecase } from "../../../src/usecase/get_token.ts";
import type { SecretData, TokenResponse } from "../../../src/domain/types.ts";
import type { SecretManager } from "../../../src/port/secret_manager.ts";

function createMockSecretManager(
  tokenData: SecretData | null = null,
  shouldFail = false,
): SecretManager {
  return {
    getTokenSecret: () => {
      if (shouldFail) {
        return Promise.reject(new Error("Storage read failed"));
      }
      return Promise.resolve(tokenData);
    },
    updateTokenSecret: (_tokens: TokenResponse) => Promise.resolve(),
    checkSecretExists: () => Promise.resolve(tokenData !== null),
  };
}

describe("GetTokenUsecase", () => {
  it("should fail fast when secretManager is not wired", () => {
    assertThrows(
      () => new GetTokenUsecase(null as unknown as SecretManager),
      Error,
      "GetTokenUsecase: secretManager is required and must be wired at composition root",
    );
  });

  it("should return token data when secret exists", async () => {
    const sampleSecret: SecretData = {
      access_token: "test-access-token",
      refresh_token: "test-refresh-token",
      expires_at: "2026-09-25T12:00:00.000Z",
      updated_at: "2026-09-24T12:00:00.000Z",
      token_type: "Bearer",
      scope: "read",
    };
    const mockManager = createMockSecretManager(sampleSecret);
    const usecase = new GetTokenUsecase(mockManager);

    const result = await usecase.execute();
    assertEquals(result, sampleSecret);
  });

  it("should return null when secret does not exist", async () => {
    const mockManager = createMockSecretManager(null);
    const usecase = new GetTokenUsecase(mockManager);

    const result = await usecase.execute();
    assertEquals(result, null);
  });

  it("should propagate error when storage fails", async () => {
    const mockManager = createMockSecretManager(null, true);
    const usecase = new GetTokenUsecase(mockManager);

    await assertRejects(
      () => usecase.execute(),
      Error,
      "Storage read failed",
    );
  });

  it("should return public token data without refresh_token via getPublicToken", async () => {
    const sampleSecret: SecretData = {
      access_token: "test-access-token",
      refresh_token: "test-refresh-token",
      expires_at: "2026-09-25T12:00:00.000Z",
      updated_at: "2026-09-24T12:00:00.000Z",
      token_type: "Bearer",
      scope: "read",
    };
    const mockManager = createMockSecretManager(sampleSecret);
    const usecase = new GetTokenUsecase(mockManager);

    const publicToken = await usecase.getPublicToken();
    assertEquals(publicToken, {
      access_token: "test-access-token",
      expires_at: "2026-09-25T12:00:00.000Z",
      updated_at: "2026-09-24T12:00:00.000Z",
      token_type: "Bearer",
      scope: "read",
    });
    assertEquals(
      (publicToken as Record<string, unknown> | null)?.refresh_token,
      undefined,
    );
  });

  it("should not pass through unknown extra fields from storage via getPublicToken allowlist", async () => {
    const rawSecretWithExtra = {
      access_token: "test-access-token",
      refresh_token: "test-refresh-token",
      expires_at: "2026-09-25T12:00:00.000Z",
      updated_at: "2026-09-24T12:00:00.000Z",
      token_type: "Bearer",
      scope: "read",
      secret_extra_key: "leaked-secret",
      internal_notes: "should-be-omitted",
    };
    const mockManager = createMockSecretManager(
      rawSecretWithExtra as unknown as SecretData,
    );
    const usecase = new GetTokenUsecase(mockManager);

    const publicToken = await usecase.getPublicToken();
    assertEquals(publicToken, {
      access_token: "test-access-token",
      expires_at: "2026-09-25T12:00:00.000Z",
      updated_at: "2026-09-24T12:00:00.000Z",
      token_type: "Bearer",
      scope: "read",
    });
    const record = publicToken as Record<string, unknown>;
    assertEquals(record.refresh_token, undefined);
    assertEquals(record.secret_extra_key, undefined);
    assertEquals(record.internal_notes, undefined);
    assertEquals(Object.keys(publicToken ?? {}).sort(), [
      "access_token",
      "expires_at",
      "scope",
      "token_type",
      "updated_at",
    ]);
  });

  it("should return null from getPublicToken when no token exists", async () => {
    const mockManager = createMockSecretManager(null);
    const usecase = new GetTokenUsecase(mockManager);

    const publicToken = await usecase.getPublicToken();
    assertEquals(publicToken, null);
  });
});
