import { describe, it } from "@std/testing/bdd";
import { assertEquals, assertThrows } from "@std/testing/asserts";
import { DaemonLoop } from "../../../src/handler/daemon.ts";
import type { RefreshTokenUsecase } from "../../../src/usecase/refresh_token.ts";
import type { GetTokenUsecase } from "../../../src/usecase/get_token.ts";
import type { OAuthServer } from "../../../src/handler/oauth_server.ts";
import type {
  AuthenticationResult,
  SecretData,
} from "../../../src/domain/types.ts";

describe("DaemonLoop", () => {
  it("should fail fast when required dependencies are missing", () => {
    const refresh = {} as RefreshTokenUsecase;
    const getToken = {} as GetTokenUsecase;
    const oauth = {} as OAuthServer;

    assertThrows(
      () =>
        new DaemonLoop(null as unknown as RefreshTokenUsecase, getToken, oauth),
      Error,
      "DaemonLoop: all dependencies (refreshUsecase, getTokenUsecase, oauthServer) are required and must be wired at composition root",
    );
    assertThrows(
      () => new DaemonLoop(refresh, null as unknown as GetTokenUsecase, oauth),
      Error,
      "DaemonLoop: all dependencies (refreshUsecase, getTokenUsecase, oauthServer) are required and must be wired at composition root",
    );
    assertThrows(
      () => new DaemonLoop(refresh, getToken, null as unknown as OAuthServer),
      Error,
      "DaemonLoop: all dependencies (refreshUsecase, getTokenUsecase, oauthServer) are required and must be wired at composition root",
    );
  });

  it("should trigger refresh when token expires in less than 2 hours", async () => {
    let refreshCalled = false;
    const mockRefresh = {
      execute: () => {
        refreshCalled = true;
        return Promise.resolve({ success: true } as AuthenticationResult);
      },
    } as unknown as RefreshTokenUsecase;

    const expiringDate = new Date(Date.now() + 60 * 60 * 1000).toISOString(); // 1 hour from now
    const mockGetToken = {
      execute: () =>
        Promise.resolve({
          access_token: "access",
          refresh_token: "valid-refresh-token",
          expires_at: expiringDate,
          updated_at: new Date().toISOString(),
        } as SecretData),
    } as unknown as GetTokenUsecase;

    const mockOAuth = {
      start: () => {},
    } as unknown as OAuthServer;

    const daemon = new DaemonLoop(mockRefresh, mockGetToken, mockOAuth);
    await (daemon as unknown as { checkAndRefreshToken(): Promise<void> })
      .checkAndRefreshToken();

    assertEquals(refreshCalled, true);
  });

  it("should skip refresh when token is still valid (> 2 hours)", async () => {
    let refreshCalled = false;
    const mockRefresh = {
      execute: () => {
        refreshCalled = true;
        return Promise.resolve({ success: true } as AuthenticationResult);
      },
    } as unknown as RefreshTokenUsecase;

    const validDate = new Date(Date.now() + 5 * 60 * 60 * 1000).toISOString(); // 5 hours from now
    const mockGetToken = {
      execute: () =>
        Promise.resolve({
          access_token: "access",
          refresh_token: "valid-refresh-token",
          expires_at: validDate,
          updated_at: new Date().toISOString(),
        } as SecretData),
    } as unknown as GetTokenUsecase;

    const mockOAuth = {
      start: () => {},
    } as unknown as OAuthServer;

    const daemon = new DaemonLoop(mockRefresh, mockGetToken, mockOAuth);
    await (daemon as unknown as { checkAndRefreshToken(): Promise<void> })
      .checkAndRefreshToken();

    assertEquals(refreshCalled, false);
  });

  it("should skip refresh when no refresh token exists", async () => {
    let refreshCalled = false;
    const mockRefresh = {
      execute: () => {
        refreshCalled = true;
        return Promise.resolve({ success: true } as AuthenticationResult);
      },
    } as unknown as RefreshTokenUsecase;

    const mockGetToken = {
      execute: () => Promise.resolve(null),
    } as unknown as GetTokenUsecase;

    const mockOAuth = {
      start: () => {},
    } as unknown as OAuthServer;

    const daemon = new DaemonLoop(mockRefresh, mockGetToken, mockOAuth);
    await (daemon as unknown as { checkAndRefreshToken(): Promise<void> })
      .checkAndRefreshToken();

    assertEquals(refreshCalled, false);
  });
});
