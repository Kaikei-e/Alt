import { describe, it } from "@std/testing/bdd";
import {
  assertEquals,
  assertExists,
  assertRejects,
} from "@std/testing/asserts";
import { InoreaderTokenClient } from "../../../src/gateway/inoreader_token_client.ts";
import type { HttpClient } from "../../../src/port/http_client.ts";
import type { InoreaderCredentials } from "../../../src/domain/types.ts";

const TEST_CREDENTIALS: InoreaderCredentials = {
  client_id: "test-client-id",
  client_secret: "test-client-secret",
  redirect_uri: "http://localhost:8080/callback",
};

function createMockHttpClient(
  responseBody: Record<string, unknown>,
  status = 200,
): HttpClient {
  return {
    fetch: () =>
      Promise.resolve(
        new Response(JSON.stringify(responseBody), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
      ),
  };
}

/** Like createMockHttpClient, but also records the url/init of every call
 * so tests can assert on exactly what was sent to the token endpoint. */
function createCapturingHttpClient(
  responseBody: Record<string, unknown>,
  status = 200,
): {
  httpClient: HttpClient;
  calls: Array<{ url: string; init: RequestInit }>;
} {
  const calls: Array<{ url: string; init: RequestInit }> = [];
  const httpClient: HttpClient = {
    fetch: (url: string, init: RequestInit = {}) => {
      calls.push({ url, init });
      return Promise.resolve(
        new Response(JSON.stringify(responseBody), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
      );
    },
  };
  return { httpClient, calls };
}

describe("InoreaderTokenClient", {
  sanitizeResources: false,
  sanitizeOps: false,
}, () => {
  it("should refresh token successfully", async () => {
    const httpClient = createMockHttpClient({
      access_token: "new-access-token-from-inoreader",
      refresh_token: "new-refresh-token-from-inoreader",
      expires_in: 3600,
      token_type: "Bearer",
      scope: "read write",
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);
    const result = await client.refreshToken("old-refresh-token");

    assertEquals(result.access_token, "new-access-token-from-inoreader");
    assertEquals(result.refresh_token, "new-refresh-token-from-inoreader");
    assertEquals(result.token_type, "Bearer");
  });

  it("should throw on invalid_grant error", async () => {
    const httpClient = createMockHttpClient(
      {
        error: "invalid_grant",
        error_description: "Token expired",
      },
      400,
    );

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("expired-token"),
      Error,
      "Invalid or expired refresh token",
    );
  });

  it("should throw on invalid_client error", async () => {
    const httpClient = createMockHttpClient(
      {
        error: "invalid_client",
        error_description: "Bad credentials",
      },
      401,
    );

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("valid-token"),
      Error,
      "Invalid client credentials",
    );
  });

  it("should throw when access_token is missing in response", async () => {
    const httpClient = createMockHttpClient({
      refresh_token: "new-refresh-token-from-inoreader",
      expires_in: 3600,
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("valid-token"),
      Error,
      "missing access_token",
    );
  });

  it("should throw when access_token is too short", async () => {
    const httpClient = createMockHttpClient({
      access_token: "short",
      refresh_token: "new-refresh-token-from-inoreader",
      expires_in: 3600,
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("valid-token"),
      Error,
      "invalid access_token format",
    );
  });

  it("should throw when refresh_token is missing in response", async () => {
    const httpClient = createMockHttpClient({
      access_token: "new-access-token-from-inoreader",
      expires_in: 3600,
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("valid-token"),
      Error,
      "missing refresh_token",
    );
  });

  it("should throw when refresh_token is too short", async () => {
    const httpClient = createMockHttpClient({
      access_token: "new-access-token-from-inoreader",
      refresh_token: "short",
      expires_in: 3600,
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("valid-token"),
      Error,
      "invalid refresh_token format",
    );
  });

  it("should throw when expires_in is missing in response", async () => {
    const httpClient = createMockHttpClient({
      access_token: "new-access-token-from-inoreader",
      refresh_token: "new-refresh-token-from-inoreader",
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.refreshToken("valid-token"),
      Error,
      "missing or invalid expires_in",
    );
  });

  it("should send grant_type=refresh_token with client credentials and the refresh token in the request body", async () => {
    const { httpClient, calls } = createCapturingHttpClient({
      access_token: "new-access-token-from-inoreader",
      refresh_token: "new-refresh-token-from-inoreader",
      expires_in: 3600,
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);
    await client.refreshToken("old-refresh-token-1234567890");

    assertEquals(calls.length, 1);
    const call = calls[0];
    assertExists(call);
    assertEquals(call.url, "https://www.inoreader.com/oauth2/token");
    assertEquals(call.init.method, "POST");

    const body = call.init.body as URLSearchParams;
    assertEquals(body.get("grant_type"), "refresh_token");
    assertEquals(body.get("client_id"), TEST_CREDENTIALS.client_id);
    assertEquals(body.get("client_secret"), TEST_CREDENTIALS.client_secret);
    assertEquals(body.get("refresh_token"), "old-refresh-token-1234567890");
  });

  it("should send grant_type=authorization_code with client credentials, redirect_uri and the code in the request body", async () => {
    const { httpClient, calls } = createCapturingHttpClient({
      access_token: "code-exchange-access-token",
      refresh_token: "code-exchange-refresh-token",
      expires_in: 3600,
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);
    await client.exchangeCode("auth-code-123");

    assertEquals(calls.length, 1);
    const call = calls[0];
    assertExists(call);
    assertEquals(call.url, "https://www.inoreader.com/oauth2/token");

    const body = call.init.body as URLSearchParams;
    assertEquals(body.get("grant_type"), "authorization_code");
    assertEquals(body.get("client_id"), TEST_CREDENTIALS.client_id);
    assertEquals(body.get("client_secret"), TEST_CREDENTIALS.client_secret);
    assertEquals(body.get("redirect_uri"), TEST_CREDENTIALS.redirect_uri);
    assertEquals(body.get("code"), "auth-code-123");
  });

  it("should exchange authorization code for tokens", async () => {
    const httpClient = createMockHttpClient({
      access_token: "code-exchange-access-token",
      refresh_token: "code-exchange-refresh-token",
      expires_in: 3600,
      token_type: "Bearer",
    });

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);
    const result = await client.exchangeCode("auth-code-123");

    assertEquals(result.access_token, "code-exchange-access-token");
    assertEquals(result.refresh_token, "code-exchange-refresh-token");
  });

  it("should throw on code exchange failure", async () => {
    const httpClient: HttpClient = {
      fetch: () =>
        Promise.resolve(
          new Response("Bad Request", { status: 400 }),
        ),
    };

    const client = new InoreaderTokenClient(TEST_CREDENTIALS, httpClient);

    await assertRejects(
      () => client.exchangeCode("invalid-code"),
      Error,
      "Token exchange failed",
    );
  });
});
