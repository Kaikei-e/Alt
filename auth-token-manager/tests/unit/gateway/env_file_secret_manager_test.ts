import { afterEach, describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { EnvFileSecretManager } from "../../../src/gateway/env_file_secret_manager.ts";
import type { TokenResponse } from "../../../src/domain/types.ts";

const TEST_DIR = "/tmp/auth-token-manager-test";

async function cleanup() {
  try {
    await Deno.remove(TEST_DIR, { recursive: true });
  } catch {
    // Ignore
  }
}

describe("EnvFileSecretManager", {
  sanitizeResources: false,
  sanitizeOps: false,
}, () => {
  afterEach(async () => {
    await cleanup();
  });

  it("should return null when file doesn't exist", async () => {
    const manager = new EnvFileSecretManager(
      `${TEST_DIR}/nonexistent.env`,
    );
    const result = await manager.getTokenSecret();
    assertEquals(result, null);
  });

  it("should write and read tokens correctly", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;
    const manager = new EnvFileSecretManager(filePath);

    const tokens: TokenResponse = {
      access_token: "test-access-token-1234567890",
      refresh_token: "test-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
      token_type: "Bearer",
      scope: "read write",
    };

    await manager.updateTokenSecret(tokens);

    const result = await manager.getTokenSecret();
    assertEquals(result?.access_token, "test-access-token-1234567890");
    assertEquals(result?.refresh_token, "test-refresh-token-1234567890");
    assertEquals(result?.token_type, "Bearer");
  });

  it("should preserve non-token env vars", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;

    await Deno.writeTextFile(
      filePath,
      "CUSTOM_VAR=custom_value\nOTHER_VAR=other\n",
    );

    const manager = new EnvFileSecretManager(filePath);

    const tokens: TokenResponse = {
      access_token: "new-access-token-1234567890",
      refresh_token: "new-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
    };

    await manager.updateTokenSecret(tokens);

    const content = await Deno.readTextFile(filePath);
    assertEquals(content.includes("CUSTOM_VAR=custom_value"), true);
    assertEquals(content.includes("OTHER_VAR=other"), true);
    assertEquals(
      content.includes("OAUTH2_ACCESS_TOKEN=new-access-token-1234567890"),
      true,
    );
  });

  it("should report existence correctly", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;

    const manager = new EnvFileSecretManager(filePath);
    assertEquals(await manager.checkSecretExists(), false);

    await Deno.writeTextFile(filePath, "test=1\n");
    assertEquals(await manager.checkSecretExists(), true);
  });

  it("should return null when required keys are missing", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;

    await Deno.writeTextFile(filePath, "SOME_KEY=some_value\n");

    const manager = new EnvFileSecretManager(filePath);
    const result = await manager.getTokenSecret();
    assertEquals(result, null);
  });

  it("should create parent directories when they don't exist", async () => {
    const filePath = `${TEST_DIR}/deep/nested/dir/tokens.env`;
    const manager = new EnvFileSecretManager(filePath);

    const tokens: TokenResponse = {
      access_token: "test-access-token-1234567890",
      refresh_token: "test-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
    };

    await manager.updateTokenSecret(tokens);

    const result = await manager.getTokenSecret();
    assertEquals(result?.access_token, "test-access-token-1234567890");
  });
});
