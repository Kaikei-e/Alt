/**
 * Daemon loop handler - manages token refresh lifecycle
 * Uses sequential execution (no setInterval overlap risk)
 */

import type { RefreshTokenUsecase } from "../usecase/refresh_token.ts";
import type { SecretManager } from "../port/secret_manager.ts";
import type { OAuthServer } from "./oauth_server.ts";
import { logger } from "../infra/logger.ts";

const CHECK_INTERVAL_MS = 5 * 60 * 1000; // 5 minutes
const TWO_HOURS_MS = 2 * 60 * 60 * 1000;

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      reject(new DOMException("Aborted", "AbortError"));
    }, { once: true });
  });
}

export class DaemonLoop {
  constructor(
    private refreshUsecase: RefreshTokenUsecase,
    private secretManager: SecretManager,
    private oauthServer: OAuthServer,
  ) {}

  async start(): Promise<void> {
    logger.info("Starting auth-token-manager in DAEMON mode");

    const ac = new AbortController();

    // Graceful shutdown
    const signals: Deno.Signal[] = ["SIGINT", "SIGTERM"];
    for (const signal of signals) {
      Deno.addSignalListener(signal, () => {
        logger.info("Received termination signal, shutting down gracefully", {
          signal,
        });
        ac.abort();
      });
    }

    // Start OAuth server in background
    try {
      this.oauthServer.start(ac.signal);
    } catch (err) {
      logger.error("Failed to start OAuth server", {
        error: err instanceof Error ? err.message : String(err),
      });
    }

    // Token monitor loop
    logger.info("Starting token monitor loop (interval: 5 minutes)");

    try {
      while (!ac.signal.aborted) {
        await this.checkAndRefreshToken();

        try {
          await delay(CHECK_INTERVAL_MS, ac.signal);
        } catch {
          // AbortError - shutdown requested
          break;
        }
      }
    } finally {
      logger.info("Daemon loop stopped");
    }
  }

  private async checkAndRefreshToken(): Promise<void> {
    try {
      const tokenData = await this.secretManager.getTokenSecret();

      if (!tokenData || !tokenData.refresh_token) {
        logger.warn(
          "No valid refresh token found - waiting for user authorization",
        );
        return;
      }

      const expiresAt = tokenData.expires_at
        ? new Date(tokenData.expires_at)
        : null;

      if (!expiresAt) {
        logger.warn(
          "Token exists but has no expiry date - refreshing",
        );
        await this.refreshUsecase.execute();
        return;
      }

      const timeUntilExpiry = expiresAt.getTime() - Date.now();

      logger.info("Token status check", {
        expires_at: expiresAt.toISOString(),
        time_until_expiry_minutes: Math.round(timeUntilExpiry / 1000 / 60),
      });

      if (timeUntilExpiry < TWO_HOURS_MS) {
        logger.info(
          "Token expiring soon (< 2 hours) - triggering refresh",
        );
        await this.refreshUsecase.execute();
      } else {
        logger.info("Token is still valid - no action needed");
      }
    } catch (err) {
      logger.error("Failed to check/refresh token", {
        error: err instanceof Error ? err.message : String(err),
      });
    }
  }
}
