/**
 * RefreshTokenUsecase - Core token refresh business logic
 */

import type { TokenClient } from "../port/token_client.ts";
import type { SecretManager } from "../port/secret_manager.ts";
import {
  INOREADER_OAUTH_SCOPE,
  type AuthenticationResult,
  type NetworkConfig,
  type RetryConfig,
  type TokenResponse,
} from "../domain/types.ts";
import type { HttpClient } from "../port/http_client.ts";
import { logger } from "../infra/logger.ts";
import { retryWithBackoff } from "../infra/retry.ts";

export class RefreshTokenUsecase {
  constructor(
    private tokenClient: TokenClient,
    private secretManager: SecretManager,
    private httpClient: HttpClient,
    private networkConfig: NetworkConfig,
    private retryConfig: RetryConfig,
  ) {}

  async execute(): Promise<AuthenticationResult> {
    const startTime = Date.now();

    try {
      logger.info("Starting token refresh");

      if (this.networkConfig.connectivity_check) {
        await this.checkNetworkConnectivity();
      }

      // Retry boundary: only the read + precondition checks are retried
      // here. They are pure reads, so retrying them is safe.
      const readResult = await retryWithBackoff(
        () => this.readExistingToken(),
        this.retryConfig,
        "read existing token",
      );

      if (readResult.type === "still_valid") {
        return {
          success: true,
          tokens: readResult.tokens,
          metadata: {
            duration: Date.now() - startTime,
            method: "refresh_token",
            session_id: crypto.randomUUID(),
          },
        };
      }

      // tokenClient.refreshToken() rotates the refresh_token server-side and
      // is therefore non-idempotent. It must NOT sit inside retryWithBackoff:
      // if the rotation succeeds but the subsequent write fails, a retry here
      // would resend the already-consumed refresh_token and fail with
      // invalid_grant, permanently losing the newly-rotated token.
      const tokens = await this.tokenClient.refreshToken(
        readResult.refreshToken,
      );

      // Only the (idempotent) persistence step is retried, using the
      // already-rotated token held in memory.
      await retryWithBackoff(
        () => this.secretManager.updateTokenSecret(tokens),
        this.retryConfig,
        "persist refreshed token",
      );

      const duration = Date.now() - startTime;
      logger.info("Token refresh completed successfully", {
        duration_ms: duration,
      });

      return {
        success: true,
        tokens,
        metadata: {
          duration,
          method: "refresh_token",
          session_id: crypto.randomUUID(),
        },
      };
    } catch (error) {
      const duration = Date.now() - startTime;
      const errorMessage = error instanceof Error
        ? error.message
        : String(error);

      logger.error("Token refresh failed", {
        duration_ms: duration,
        error: errorMessage,
      });

      return {
        success: false,
        error: errorMessage,
        metadata: {
          duration,
          method: "refresh_token",
          session_id: crypto.randomUUID(),
        },
      };
    }
  }

  private async readExistingToken(): Promise<
    | { type: "still_valid"; tokens: TokenResponse }
    | { type: "needs_refresh"; refreshToken: string }
  > {
    const existingTokenData = await this.secretManager.getTokenSecret();
    if (!existingTokenData || !existingTokenData.refresh_token) {
      throw new Error(
        "No existing refresh token found. Manual OAuth setup required.",
      );
    }

    if (existingTokenData.refresh_token.length < 10) {
      throw new Error(
        "Invalid refresh token format. Manual OAuth setup required.",
      );
    }

    // Check if current token is still valid (within 5 minutes of expiry)
    if (existingTokenData.expires_at) {
      const expiresAt = new Date(existingTokenData.expires_at);
      const timeUntilExpiry = expiresAt.getTime() - Date.now();
      const fiveMinutes = 5 * 60 * 1000;

      if (timeUntilExpiry > fiveMinutes) {
        logger.info("Current access token is still valid", {
          time_until_expiry_minutes: Math.round(
            timeUntilExpiry / 1000 / 60,
          ),
        });
        return {
          type: "still_valid",
          tokens: {
            access_token: existingTokenData.access_token,
            refresh_token: existingTokenData.refresh_token,
            expires_at: expiresAt,
            token_type: existingTokenData.token_type || "Bearer",
            scope: existingTokenData.scope || INOREADER_OAUTH_SCOPE,
          },
        };
      }
    }

    return {
      type: "needs_refresh",
      refreshToken: existingTokenData.refresh_token,
    };
  }

  private async checkNetworkConnectivity(): Promise<void> {
    try {
      const response = await this.httpClient.fetch(
        "https://www.inoreader.com",
        { method: "HEAD" },
      );

      try {
        if (!response.ok && response.status >= 500) {
          throw new Error(`Inoreader server error: ${response.status}`);
        }
      } finally {
        // Release the connection; HEAD bodies are empty but cancel is still required.
        await response.body?.cancel();
      }
    } catch (error) {
      const errorMessage = error instanceof Error
        ? error.message
        : String(error);
      throw new Error(`Network connectivity check failed: ${errorMessage}`);
    }
  }
}
