/**
 * HealthCheckUsecase - Health check business logic
 */

import type { SecretManager } from "../port/secret_manager.ts";
import type { HealthChecks, HealthStatus } from "../domain/types.ts";
import { config } from "../infra/config.ts";
import { logger } from "../infra/logger.ts";

export interface HealthCheckResult {
  status: HealthStatus;
  checks: HealthChecks;
  passing: number;
  total: number;
}

export class HealthCheckUsecase {
  constructor(private secretManager: SecretManager) {}

  async execute(): Promise<HealthCheckResult> {
    logger.info("Running health check");

    const checks: HealthChecks = {
      config_valid: false,
      environment_ready: false,
      storage_ready: false,
      refresh_token_available: false,
      token_expiry_status: false,
    };

    checks.config_valid = config.validateConfig();

    checks.environment_ready = Boolean(
      Deno.env.get("INOREADER_CLIENT_ID") &&
        Deno.env.get("INOREADER_CLIENT_SECRET"),
    );

    try {
      const tokenData = await this.secretManager.getTokenSecret();
      checks.storage_ready = true;

      if (tokenData && tokenData.refresh_token) {
        checks.refresh_token_available = tokenData.refresh_token.length > 10;

        if (tokenData.expires_at) {
          const expiresAt = new Date(tokenData.expires_at);
          const timeUntilExpiry = expiresAt.getTime() - Date.now();
          const oneHour = 60 * 60 * 1000;
          checks.token_expiry_status = timeUntilExpiry > oneHour;
        }
      }
    } catch (error) {
      logger.warn("Storage check failed", {
        error: error instanceof Error ? error.message : String(error),
      });
    }

    const healthyChecks = Object.values(checks).filter(Boolean).length;
    const totalChecks = Object.keys(checks).length;

    let status: HealthStatus;
    if (healthyChecks === totalChecks) {
      status = "healthy";
    } else if (healthyChecks >= 3) {
      status = "degraded";
    } else {
      status = "unhealthy";
    }

    logger.info("Health check completed", {
      status,
      passing: healthyChecks,
      total: totalChecks,
      checks,
    });

    return { status, checks, passing: healthyChecks, total: totalChecks };
  }
}
