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

  it("should correctly read token values containing = characters", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;

    // Base64-encoded tokens often contain = padding characters
    const accessToken = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0=";
    const refreshToken = "dGVzdC1yZWZyZXNoLXRva2Vu==";

    await Deno.writeTextFile(
      filePath,
      `OAUTH2_ACCESS_TOKEN=${accessToken}\nOAUTH2_REFRESH_TOKEN=${refreshToken}\nOAUTH2_TOKEN_TYPE=Bearer\nOAUTH2_EXPIRES_AT=2025-12-31T00:00:00.000Z\n`,
    );

    const manager = new EnvFileSecretManager(filePath);
    const result = await manager.getTokenSecret();

    assertEquals(result?.access_token, accessToken);
    assertEquals(result?.refresh_token, refreshToken);
  });

  it("should write and read back tokens with = in values via round-trip", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;
    const manager = new EnvFileSecretManager(filePath);

    const accessToken = "token_with_base64_padding=";
    const refreshToken = "refresh_with_double_pad==";

    const tokens: TokenResponse = {
      access_token: accessToken,
      refresh_token: refreshToken,
      expires_at: new Date(Date.now() + 3600 * 1000),
      token_type: "Bearer",
    };

    await manager.updateTokenSecret(tokens);

    const result = await manager.getTokenSecret();
    assertEquals(result?.access_token, accessToken);
    assertEquals(result?.refresh_token, refreshToken);
  });

  it("should persist OAUTH2_UPDATED_AT on write and return the persisted value on read (not read-time)", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;
    const manager = new EnvFileSecretManager(filePath);

    const tokens: TokenResponse = {
      access_token: "test-access-token-1234567890",
      refresh_token: "test-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
    };

    await manager.updateTokenSecret(tokens);

    const first = await manager.getTokenSecret();
    await new Promise((resolve) => setTimeout(resolve, 20));
    const second = await manager.getTokenSecret();

    assertEquals(typeof first?.updated_at, "string");
    assertEquals(first?.updated_at !== "", true);
    // Reading twice must return the same persisted write time, not the
    // current read time each call.
    assertEquals(first?.updated_at, second?.updated_at);

    const content = await Deno.readTextFile(filePath);
    assertEquals(content.includes("OAUTH2_UPDATED_AT="), true);
  });

  it("should update OAUTH2_UPDATED_AT (not duplicate it) on a subsequent write", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;
    const manager = new EnvFileSecretManager(filePath);

    await manager.updateTokenSecret({
      access_token: "first-access-token-1234567890",
      refresh_token: "first-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
    });
    const first = await manager.getTokenSecret();

    await new Promise((resolve) => setTimeout(resolve, 20));

    await manager.updateTokenSecret({
      access_token: "second-access-token-1234567890",
      refresh_token: "second-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
    });
    const second = await manager.getTokenSecret();

    assertEquals(second?.updated_at !== first?.updated_at, true);

    const content = await Deno.readTextFile(filePath);
    const matches = content.match(/OAUTH2_UPDATED_AT=/g) ?? [];
    assertEquals(matches.length, 1);
  });

  it("should set file permissions to 0o640 after writing", async () => {
    await Deno.mkdir(TEST_DIR, { recursive: true });
    const filePath = `${TEST_DIR}/tokens.env`;
    const manager = new EnvFileSecretManager(filePath);

    const tokens: TokenResponse = {
      access_token: "test-access-token-1234567890",
      refresh_token: "test-refresh-token-1234567890",
      expires_at: new Date(Date.now() + 3600 * 1000),
    };

    await manager.updateTokenSecret(tokens);

    const stat = await Deno.stat(filePath);
    // 0o640 = owner rw, group r, others none; mask off file type bits
    assertEquals(stat.mode! & 0o777, 0o640);
  });
});
