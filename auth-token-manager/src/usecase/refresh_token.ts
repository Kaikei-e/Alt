/**
 * RefreshTokenUsecase - Core token refresh business logic
 */

import type { TokenClient } from "../port/token_client.ts";
import type { SecretManager } from "../port/secret_manager.ts";
import type {
  AuthenticationResult,
  NetworkConfig,
  RetryConfig,
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

      return await retryWithBackoff(
        async () => {
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
                success: true,
                tokens: {
                  access_token: existingTokenData.access_token,
                  refresh_token: existingTokenData.refresh_token,
                  expires_at: expiresAt,
                  token_type: "Bearer",
                  scope: "read write",
                },
                metadata: {
                  duration: Date.now() - startTime,
                  method: "refresh_token",
                  session_id: crypto.randomUUID(),
                },
              };
            }
          }

          const tokens = await this.tokenClient.refreshToken(
            existingTokenData.refresh_token,
          );

          await this.secretManager.updateTokenSecret(tokens);

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
        },
        this.retryConfig,
        "refresh token flow",
      );
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

  private async checkNetworkConnectivity(): Promise<void> {
    try {
      const response = await this.httpClient.fetch(
        "https://www.inoreader.com",
        { method: "HEAD" },
      );

      if (!response.ok && response.status >= 500) {
        throw new Error(`Inoreader server error: ${response.status}`);
      }
    } catch (error) {
      const errorMessage = error instanceof Error
        ? error.message
        : String(error);
      throw new Error(`Network connectivity check failed: ${errorMessage}`);
    }
  }
}
