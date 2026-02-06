/**
 * Configuration management for auth-token-manager
 */

import type { ConfigOptions, InoreaderCredentials } from "../domain/types.ts";

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
        max_attempts: parseInt(Deno.env.get("RETRY_MAX_ATTEMPTS") || "3"),
        base_delay: parseInt(Deno.env.get("RETRY_BASE_DELAY") || "1000"),
        max_delay: parseInt(Deno.env.get("RETRY_MAX_DELAY") || "30000"),
        backoff_factor: parseFloat(
          Deno.env.get("RETRY_BACKOFF_FACTOR") || "2",
        ),
      },
      network: {
        http_timeout: parseInt(Deno.env.get("HTTP_TIMEOUT") || "30000"),
        connectivity_check: Deno.env.get("CONNECTIVITY_CHECK") !== "false",
        connectivity_timeout: parseInt(
          Deno.env.get("CONNECTIVITY_TIMEOUT") || "10000",
        ),
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

    for (const env of required) {
      const value = this.getEnvOrFile(env);
      if (!value) {
        return false;
      }
      if (
        value === "demo-client-id" || value === "demo-client-secret" ||
        value === "placeholder"
      ) {
        return false;
      }
      if (value.length < 5) {
        return false;
      }
    }

    return true;
  }

  getInoreaderCredentials(): InoreaderCredentials {
    return {
      client_id: this.getEnvOrFile("INOREADER_CLIENT_ID")!,
      client_secret: this.getEnvOrFile("INOREADER_CLIENT_SECRET")!,
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
      } catch {
        // File not found or unreadable
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
