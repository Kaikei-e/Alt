/**
 * Configuration management for auth-token-manager
 */

import type { ConfigOptions, InoreaderCredentials } from "../domain/types.ts";
import { logger } from "./logger.ts";

function parseIntEnv(envVar: string, fallback: number, min = 0): number {
  const raw = Deno.env.get(envVar);
  if (raw === undefined) return fallback;

  const parsed = parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed < min) {
    logger.warn(`Invalid ${envVar} value, falling back to default`, {
      value: raw,
      default: fallback,
    });
    return fallback;
  }
  return parsed;
}

function parseFloatEnv(envVar: string, fallback: number, min = 0): number {
  const raw = Deno.env.get(envVar);
  if (raw === undefined) return fallback;

  const parsed = parseFloat(raw);
  if (!Number.isFinite(parsed) || parsed < min) {
    logger.warn(`Invalid ${envVar} value, falling back to default`, {
      value: raw,
      default: fallback,
    });
    return fallback;
  }
  return parsed;
}

class ConfigManager {
  private static instance: ConfigManager;
  private configCache: ConfigOptions | null = null;

  static getInstance(): ConfigManager {
    if (!this.instance) {
      this.instance = new ConfigManager();
    }
    return this.instance;
  }

  get tokenStoragePath(): string {
    return this.configCache?.token_storage_path ??
      (Deno.env.get("TOKEN_STORAGE_PATH") || "/app/secrets/oauth2_token.env");
  }

  loadConfig(): Promise<ConfigOptions> {
    if (this.configCache) {
      return Promise.resolve(this.configCache);
    }

    this.configCache = {
      token_storage_path: Deno.env.get("TOKEN_STORAGE_PATH") ||
        "/app/secrets/oauth2_token.env",
      retry: {
        max_attempts: parseIntEnv("RETRY_MAX_ATTEMPTS", 3, 1),
        base_delay: parseIntEnv("RETRY_BASE_DELAY", 1000, 0),
        max_delay: parseIntEnv("RETRY_MAX_DELAY", 30000, 0),
        backoff_factor: parseFloatEnv("RETRY_BACKOFF_FACTOR", 2, 0),
      },
      network: {
        http_timeout: parseIntEnv("HTTP_TIMEOUT", 30000, 0),
        connectivity_check: Deno.env.get("CONNECTIVITY_CHECK") !== "false",
        connectivity_timeout: parseIntEnv("CONNECTIVITY_TIMEOUT", 10000, 0),
      },
      logger: {
        level: (Deno.env.get("LOG_LEVEL") || "INFO").toUpperCase(),
        include_timestamp: Deno.env.get("LOG_INCLUDE_TIMESTAMP") !== "false",
        include_stack_trace:
          Deno.env.get("LOG_INCLUDE_STACK_TRACE") !== "false",
      },
    };

    return Promise.resolve(this.configCache);
  }

  validateConfig(): boolean {
    const required = [
      "INOREADER_CLIENT_ID",
      "INOREADER_CLIENT_SECRET",
    ];
    const failed: string[] = [];

    for (const env of required) {
      const value = this.getEnvOrFile(env);
      if (!value) {
        failed.push(`${env} (missing)`);
        continue;
      }
      if (
        value === "demo-client-id" || value === "demo-client-secret" ||
        value === "placeholder"
      ) {
        failed.push(`${env} (placeholder)`);
        continue;
      }
      if (value.length < 5) {
        failed.push(`${env} (too short)`);
      }
    }

    if (failed.length > 0) {
      // Log variable names only — never values.
      logger.error("Configuration validation failed", {
        failed_variables: failed,
      });
      return false;
    }

    return true;
  }

  getInoreaderCredentials(): InoreaderCredentials {
    const client_id = this.getEnvOrFile("INOREADER_CLIENT_ID");
    if (!client_id) {
      throw new Error(
        "INOREADER_CLIENT_ID (or INOREADER_CLIENT_ID_FILE) is not configured",
      );
    }

    const client_secret = this.getEnvOrFile("INOREADER_CLIENT_SECRET");
    if (!client_secret) {
      throw new Error(
        "INOREADER_CLIENT_SECRET (or INOREADER_CLIENT_SECRET_FILE) is not configured",
      );
    }

    return {
      client_id,
      client_secret,
      redirect_uri: Deno.env.get("INOREADER_REDIRECT_URI") ||
        "http://localhost:8080/callback",
    };
  }

  isProductionMode(): boolean {
    const env = Deno.env.get("NODE_ENV") || Deno.env.get("DENO_ENV") ||
      "development";
    return env === "production";
  }

  getEnvOrFile(key: string): string | undefined {
    const val = Deno.env.get(key);
    if (val) return val;

    const filePath = Deno.env.get(`${key}_FILE`);
    if (filePath) {
      try {
        return Deno.readTextFileSync(filePath).trim();
      } catch (error) {
        if (error instanceof Deno.errors.NotFound) {
          return undefined;
        }
        // Permission / other IO errors: surface for diagnosis (no secret value).
        logger.warn(`Failed to read secrets file for ${key}`, {
          file: filePath,
          error: error instanceof Error ? error.message : String(error),
        });
      }
    }

    return undefined;
  }

  /** Reset cached config (for testing) */
  resetCache(): void {
    this.configCache = null;
  }
}

export const config = ConfigManager.getInstance();
export { ConfigManager };
