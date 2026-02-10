import { afterAll, afterEach, beforeAll, describe, it } from "@std/testing/bdd";
import { assertEquals } from "@std/testing/asserts";
import { OAuthServer } from "../../../src/handler/oauth_server.ts";
import { AuthorizeUsecase } from "../../../src/usecase/authorize.ts";
import type {
  SecretData,
  SecretManager,
  TokenResponse,
} from "../../../src/domain/types.ts";
import type { TokenClient } from "../../../src/port/token_client.ts";

class StubTokenClient implements TokenClient {
  async refreshToken(_refreshToken: string): Promise<TokenResponse> {
    return {
      access_token: "stub",
      refresh_token: "stub",
      expires_at: new Date(),
    };
  }
  async exchangeCode(_code: string): Promise<TokenResponse> {
    return {
      access_token: "stub",
      refresh_token: "stub",
      expires_at: new Date(),
    };
  }
}

class StubSecretManager implements SecretManager {
  private data: SecretData | null = null;

  setData(data: SecretData | null) {
    this.data = data;
  }

  async updateTokenSecret(_tokens: TokenResponse): Promise<void> {}
  async getTokenSecret(): Promise<SecretData | null> {
    return this.data;
  }
  async checkSecretExists(): Promise<boolean> {
    return this.data !== null;
  }
}

const TEST_PORT = 19876;

describe("OAuthServer /api/token", {
  sanitizeResources: false,
  sanitizeOps: false,
}, () => {
  let controller: AbortController;
  let secretManager: StubSecretManager;

  beforeAll(() => {
    // Ensure INTERNAL_AUTH_TOKEN is unset for these tests
    Deno.env.delete("INTERNAL_AUTH_TOKEN");

    secretManager = new StubSecretManager();
    const tokenClient = new StubTokenClient();
    const credentials = {
      client_id: "test-client-id",
      client_secret: "test-client-secret",
      redirect_uri: `http://localhost:${TEST_PORT}/callback`,
    };
    const authorizeUsecase = new AuthorizeUsecase(
      tokenClient,
      secretManager,
      credentials,
    );
    const server = new OAuthServer(
      authorizeUsecase,
      secretManager,
      credentials,
    );

    controller = new AbortController();
    server.start(controller.signal);
  });

  afterAll(() => {
    controller.abort();
  });

  afterEach(() => {
    Deno.env.delete("INTERNAL_AUTH_TOKEN");
  });

  it("should return full token data including access_token when INTERNAL_AUTH_TOKEN is not set", async () => {
    secretManager.setData({
      access_token: "test-access-token-xyz",
      refresh_token: "test-refresh-token-abc",
      expires_at: "2025-12-31T00:00:00.000Z",
      updated_at: "2025-06-01T00:00:00.000Z",
      token_type: "Bearer",
      scope: "read write",
    });

    const res = await fetch(`http://localhost:${TEST_PORT}/api/token`);
    assertEquals(res.status, 200);

    const body = await res.json();
    assertEquals(body.access_token, "test-access-token-xyz");
    assertEquals(body.refresh_token, "test-refresh-token-abc");
    assertEquals(body.token_type, "Bearer");
    assertEquals(body.expires_at, "2025-12-31T00:00:00.000Z");
  });

  it("should return 404 when no token data exists", async () => {
    secretManager.setData(null);

    const res = await fetch(`http://localhost:${TEST_PORT}/api/token`);
    assertEquals(res.status, 404);

    const body = await res.json();
    assertEquals(body.error, "No token data found");
  });

  it("should return 401 when INTERNAL_AUTH_TOKEN is set and wrong header is sent", async () => {
    Deno.env.set("INTERNAL_AUTH_TOKEN", "correct-token");

    secretManager.setData({
      access_token: "test-access-token",
      refresh_token: "test-refresh-token",
      expires_at: "2025-12-31T00:00:00.000Z",
      updated_at: "2025-06-01T00:00:00.000Z",
      token_type: "Bearer",
    });

    const res = await fetch(`http://localhost:${TEST_PORT}/api/token`, {
      headers: { "X-Internal-Auth": "wrong-token" },
    });
    assertEquals(res.status, 401);
  });

  it("should return full token data when INTERNAL_AUTH_TOKEN is set and correct header is sent", async () => {
    Deno.env.set("INTERNAL_AUTH_TOKEN", "correct-token");

    secretManager.setData({
      access_token: "secret-access-token",
      refresh_token: "secret-refresh-token",
      expires_at: "2025-12-31T00:00:00.000Z",
      updated_at: "2025-06-01T00:00:00.000Z",
      token_type: "Bearer",
    });

    const res = await fetch(`http://localhost:${TEST_PORT}/api/token`, {
      headers: { "X-Internal-Auth": "correct-token" },
    });
    assertEquals(res.status, 200);

    const body = await res.json();
    assertEquals(body.access_token, "secret-access-token");
    assertEquals(body.refresh_token, "secret-refresh-token");
  });
});
