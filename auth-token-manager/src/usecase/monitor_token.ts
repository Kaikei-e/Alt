/**
 * MonitorTokenUsecase - Token monitoring and alerting logic
 */

import type { SecretManager } from "../port/secret_manager.ts";
import type { AlertLevel, MonitoringData } from "../domain/types.ts";
import { config } from "../infra/config.ts";
import { logger } from "../infra/logger.ts";

export interface MonitorResult {
  alertLevel: AlertLevel;
  monitoringData: MonitoringData;
}

export class MonitorTokenUsecase {
  constructor(private secretManager: SecretManager) {}

  async execute(): Promise<MonitorResult> {
    logger.info("Running token monitoring and alerting check");

    const tokenData = await this.secretManager.getTokenSecret();
    if (!tokenData) {
      logger.error("No token data found - OAuth setup required", {
        alert_level: "critical",
      });
      return {
        alertLevel: "critical",
        monitoringData: this.buildEmptyMonitoringData("critical", [
          "No token data found - OAuth setup required",
        ]),
      };
    }

    const now = new Date();
    const updatedAt = tokenData.updated_at
      ? new Date(tokenData.updated_at)
      : null;
    const expiresAt = tokenData.expires_at
      ? new Date(tokenData.expires_at)
      : null;

    let timeUntilExpiry = 0;
    let timeSinceUpdate = 0;

    if (expiresAt) {
      timeUntilExpiry = expiresAt.getTime() - now.getTime();
    }
    if (updatedAt) {
      timeSinceUpdate = now.getTime() - updatedAt.getTime();
    }

    const alerts: string[] = [];
    let alertLevel: AlertLevel = "info";

    const fiveMinutes = 5 * 60 * 1000;
    const thirtyMinutes = 30 * 60 * 1000;
    const oneHour = 60 * 60 * 1000;
    const twoHours = 2 * 60 * 60 * 1000;
    const sixHours = 6 * 60 * 60 * 1000;

    if (timeUntilExpiry <= 0) {
      alerts.push("Token has already expired - immediate refresh required");
      alertLevel = "critical";
    } else if (timeUntilExpiry < fiveMinutes) {
      alerts.push(
        "Token expires in less than 5 minutes - immediate refresh required",
      );
      alertLevel = "critical";
    } else if (timeUntilExpiry < thirtyMinutes) {
      alerts.push(
        "Token expires in less than 30 minutes - urgent refresh recommended",
      );
      alertLevel = "critical";
    } else if (timeUntilExpiry < oneHour) {
      alerts.push(
        "Token expires in less than 1 hour - refresh recommended",
      );
      alertLevel = "warning";
    } else if (timeUntilExpiry < twoHours) {
      alerts.push(
        "Token expires in less than 2 hours - proactive refresh recommended",
      );
      if (alertLevel === "info") alertLevel = "warning";
    } else if (timeUntilExpiry < sixHours) {
      alerts.push(
        "Token expires in less than 6 hours - preparation recommended",
      );
    }

    const oneDayMs = 24 * 60 * 60 * 1000;
    const twelveHoursMs = 12 * 60 * 60 * 1000;
    const sixHoursMs = 6 * 60 * 60 * 1000;

    if (timeSinceUpdate > oneDayMs) {
      alerts.push(
        "Token hasn't been updated in over 24 hours - system may be failing",
      );
      alertLevel = "critical";
    } else if (timeSinceUpdate > twelveHoursMs) {
      alerts.push(
        "Token hasn't been updated in over 12 hours - check CronJob status",
      );
      if (alertLevel === "info") alertLevel = "warning";
    } else if (timeSinceUpdate > sixHoursMs) {
      alerts.push(
        "Token hasn't been updated in over 6 hours - monitoring recommended",
      );
    }

    if (
      !tokenData.refresh_token || tokenData.refresh_token.length < 10
    ) {
      alerts.push(
        "Invalid or missing refresh token - manual OAuth setup required",
      );
      alertLevel = "critical";
    }

    const monitoringData: MonitoringData = {
      timestamp: now.toISOString(),
      alert_level: alertLevel,
      alerts,
      token_status: {
        has_access_token: Boolean(tokenData.access_token),
        has_refresh_token: Boolean(tokenData.refresh_token),
        expires_at: expiresAt?.toISOString() || null,
        updated_at: updatedAt?.toISOString() || null,
        time_until_expiry_hours: expiresAt
          ? Math.round(timeUntilExpiry / 1000 / 3600 * 100) / 100
          : null,
        time_since_update_hours: updatedAt
          ? Math.round(timeSinceUpdate / 1000 / 3600 * 100) / 100
          : null,
        needs_immediate_refresh: timeUntilExpiry < fiveMinutes,
        needs_refresh_soon: timeUntilExpiry < thirtyMinutes,
      },
      system_status: {
        secret_exists: true,
        configuration_valid: config.validateConfig(),
      },
    };

    if (alertLevel === "critical") {
      logger.error(
        "Token monitoring - CRITICAL alerts detected",
        monitoringData,
      );
    } else if (alertLevel === "warning") {
      logger.warn(
        "Token monitoring - WARNING alerts detected",
        monitoringData,
      );
    } else {
      logger.info(
        "Token monitoring - All systems operational",
        monitoringData,
      );
    }

    return { alertLevel, monitoringData };
  }

  private buildEmptyMonitoringData(
    alertLevel: AlertLevel,
    alerts: string[],
  ): MonitoringData {
    return {
      timestamp: new Date().toISOString(),
      alert_level: alertLevel,
      alerts,
      token_status: {
        has_access_token: false,
        has_refresh_token: false,
        expires_at: null,
        updated_at: null,
        time_until_expiry_hours: null,
        time_since_update_hours: null,
        needs_immediate_refresh: true,
        needs_refresh_soon: true,
      },
      system_status: {
        secret_exists: false,
        configuration_valid: config.validateConfig(),
      },
    };
  }
}
