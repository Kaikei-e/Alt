import { afterEach, beforeEach, describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { ConfigManager } from "../../../src/infra/config.ts";

describe(
  "ConfigManager",
  { sanitizeResources: false, sanitizeOps: false },
  () => {
    let originalEnv: Record<string, string | undefined>;

    beforeEach(() => {
      originalEnv = {
        INOREADER_CLIENT_ID: Deno.env.get("INOREADER_CLIENT_ID"),
        INOREADER_CLIENT_SECRET: Deno.env.get("INOREADER_CLIENT_SECRET"),
        INOREADER_REDIRECT_URI: Deno.env.get("INOREADER_REDIRECT_URI"),
        TOKEN_STORAGE_PATH: Deno.env.get("TOKEN_STORAGE_PATH"),
        RETRY_MAX_ATTEMPTS: Deno.env.get("RETRY_MAX_ATTEMPTS"),
        LOG_LEVEL: Deno.env.get("LOG_LEVEL"),
        NODE_ENV: Deno.env.get("NODE_ENV"),
        DENO_ENV: Deno.env.get("DENO_ENV"),
      };
    });

    afterEach(() => {
      for (const [key, value] of Object.entries(originalEnv)) {
        if (value === undefined) {
          Deno.env.delete(key);
        } else {
          Deno.env.set(key, value);
        }
      }
      // Reset singleton cache
      ConfigManager.getInstance().resetCache();
    });

    it("should validate config when required env vars are set", () => {
      Deno.env.set("INOREADER_CLIENT_ID", "valid-client-id-12345");
      Deno.env.set("INOREADER_CLIENT_SECRET", "valid-client-secret-12345");

      const config = ConfigManager.getInstance();
      assertEquals(config.validateConfig(), true);
    });

    it("should fail validation when env vars are missing", () => {
      Deno.env.delete("INOREADER_CLIENT_ID");
      Deno.env.delete("INOREADER_CLIENT_SECRET");

      const config = ConfigManager.getInstance();
      assertEquals(config.validateConfig(), false);
    });

    it("should reject placeholder values", () => {
      Deno.env.set("INOREADER_CLIENT_ID", "demo-client-id");
      Deno.env.set("INOREADER_CLIENT_SECRET", "valid-client-secret-12345");

      const config = ConfigManager.getInstance();
      assertEquals(config.validateConfig(), false);
    });

    it("should reject too-short values", () => {
      Deno.env.set("INOREADER_CLIENT_ID", "ab");
      Deno.env.set("INOREADER_CLIENT_SECRET", "valid-client-secret-12345");

      const config = ConfigManager.getInstance();
      assertEquals(config.validateConfig(), false);
    });

    it("should return credentials from env", () => {
      Deno.env.set("INOREADER_CLIENT_ID", "test-client-id-12345");
      Deno.env.set("INOREADER_CLIENT_SECRET", "test-secret-12345");
      Deno.env.set(
        "INOREADER_REDIRECT_URI",
        "http://localhost:9090/callback",
      );

      const config = ConfigManager.getInstance();
      const creds = config.getInoreaderCredentials();

      assertEquals(creds.client_id, "test-client-id-12345");
      assertEquals(creds.client_secret, "test-secret-12345");
      assertEquals(creds.redirect_uri, "http://localhost:9090/callback");
    });

    it("should use default redirect URI when not set", () => {
      Deno.env.set("INOREADER_CLIENT_ID", "test-client-id-12345");
      Deno.env.set("INOREADER_CLIENT_SECRET", "test-secret-12345");
      Deno.env.delete("INOREADER_REDIRECT_URI");

      const config = ConfigManager.getInstance();
      const creds = config.getInoreaderCredentials();

      assertEquals(creds.redirect_uri, "http://localhost:8080/callback");
    });

    it("should load config with defaults", async () => {
      const config = ConfigManager.getInstance();
      config.resetCache();
      const opts = await config.loadConfig();

      assertEquals(opts.retry.max_attempts, 3);
      assertEquals(opts.retry.base_delay, 1000);
      assertEquals(opts.network.http_timeout, 30000);
      assertEquals(opts.network.connectivity_check, true);
    });

    it("should load config from env overrides", async () => {
      Deno.env.set("RETRY_MAX_ATTEMPTS", "5");
      Deno.env.set("HTTP_TIMEOUT", "60000");
      Deno.env.set("LOG_LEVEL", "debug");

      const config = ConfigManager.getInstance();
      config.resetCache();
      const opts = await config.loadConfig();

      assertEquals(opts.retry.max_attempts, 5);
      assertEquals(opts.network.http_timeout, 60000);
      assertEquals(opts.logger.level, "DEBUG");
    });

    it("should detect production mode", () => {
      const config = ConfigManager.getInstance();

      Deno.env.delete("NODE_ENV");
      Deno.env.delete("DENO_ENV");
      assertEquals(config.isProductionMode(), false);

      Deno.env.set("NODE_ENV", "production");
      assertEquals(config.isProductionMode(), true);

      Deno.env.delete("NODE_ENV");
      Deno.env.set("DENO_ENV", "production");
      assertEquals(config.isProductionMode(), true);
    });

    it("should cache config after first load", async () => {
      const config = ConfigManager.getInstance();
      config.resetCache();

      Deno.env.set("RETRY_MAX_ATTEMPTS", "7");
      const opts1 = await config.loadConfig();
      assertEquals(opts1.retry.max_attempts, 7);

      // Change env var, but cache should return same value
      Deno.env.set("RETRY_MAX_ATTEMPTS", "10");
      const opts2 = await config.loadConfig();
      assertEquals(opts2.retry.max_attempts, 7); // cached
    });
  },
);
