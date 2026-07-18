/**
 * Inoreader OAuth token client gateway
 */

import type { TokenClient } from "../port/token_client.ts";
import type { HttpClient } from "../port/http_client.ts";
import {
  INOREADER_OAUTH_SCOPE,
  type InoreaderCredentials,
  type TokenResponse,
} from "../domain/types.ts";
import { logger } from "../infra/logger.ts";
import { PermanentError } from "../infra/retry.ts";

const INOREADER_TOKEN_URL = "https://www.inoreader.com/oauth2/token";

const TOKEN_REQUEST_HEADERS = {
  "Content-Type": "application/x-www-form-urlencoded",
  "User-Agent": "Auth-Token-Manager/2.0.0",
} as const;

// Trust-boundary narrowing for the token endpoint's JSON body: only proves
// it's an indexable object, not that the expected fields are present or
// well-typed — the field-level checks below report which specific field is
// missing/invalid rather than a single generic rejection.
interface TokenApiResponse {
  access_token?: unknown;
  refresh_token?: unknown;
  expires_in?: unknown;
  token_type?: unknown;
  scope?: unknown;
}

function isTokenApiResponse(data: unknown): data is TokenApiResponse {
  return typeof data === "object" && data !== null;
}

/** Reads the response body once as text, then parses it as JSON, converting
 * non-JSON bodies (e.g. a proxy's HTML error page) into a descriptive error
 * instead of letting SyntaxError leak out. */
async function parseJsonResponse(response: Response): Promise<unknown> {
  const text = await response.text();
  try {
    return JSON.parse(text);
  } catch {
    throw new Error(
      `Invalid token endpoint response (not JSON): ${text.slice(0, 200)}`,
    );
  }
}

export class InoreaderTokenClient implements TokenClient {
  constructor(
    private credentials: InoreaderCredentials,
    private httpClient: HttpClient,
  ) {}

  async refreshToken(refreshToken: string): Promise<TokenResponse> {
    logger.info("Requesting token refresh from Inoreader");

    const response = await this.httpClient.fetch(INOREADER_TOKEN_URL, {
      method: "POST",
      headers: { ...TOKEN_REQUEST_HEADERS },
      body: new URLSearchParams({
        grant_type: "refresh_token",
        client_id: this.credentials.client_id,
        client_secret: this.credentials.client_secret,
        refresh_token: refreshToken,
      }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      let errorData: Record<string, string> = {};
      try {
        errorData = JSON.parse(errorText);
      } catch {
        // Not JSON
      }

      const errorCode = errorData.error || "unknown_error";
      const errorDescription = errorData.error_description || errorText;

      switch (errorCode) {
        case "invalid_grant":
          throw new PermanentError(
            `Invalid or expired refresh token. Manual OAuth setup required. Details: ${errorDescription}`,
          );
        case "invalid_client":
          throw new PermanentError(
            `Invalid client credentials. Check CLIENT_ID and CLIENT_SECRET. Details: ${errorDescription}`,
          );
        case "unsupported_grant_type":
          throw new PermanentError(
            `Unsupported grant type. OAuth configuration error. Details: ${errorDescription}`,
          );
        case "invalid_request":
          throw new PermanentError(
            `Invalid OAuth request format. Details: ${errorDescription}`,
          );
        default:
          throw new Error(
            `OAuth refresh failed [${errorCode}]: ${errorDescription} (Status: ${response.status})`,
          );
      }
    }

    const data = await parseJsonResponse(response);
    return this.validateAndBuildTokenResponse(data);
  }

  async exchangeCode(code: string): Promise<TokenResponse> {
    logger.info("Exchanging authorization code for tokens");

    const response = await this.httpClient.fetch(INOREADER_TOKEN_URL, {
      method: "POST",
      headers: { ...TOKEN_REQUEST_HEADERS },
      body: new URLSearchParams({
        grant_type: "authorization_code",
        client_id: this.credentials.client_id,
        client_secret: this.credentials.client_secret,
        redirect_uri: this.credentials.redirect_uri,
        code,
      }),
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(`Token exchange failed: ${errText}`);
    }

    const data = await parseJsonResponse(response);
    return this.validateAndBuildTokenResponse(data);
  }

  private validateAndBuildTokenResponse(data: unknown): TokenResponse {
    if (!isTokenApiResponse(data)) {
      throw new Error("OAuth response is not a valid JSON object");
    }
    if (typeof data.access_token !== "string" || !data.access_token) {
      throw new Error("OAuth response missing access_token");
    }
    if (typeof data.refresh_token !== "string" || !data.refresh_token) {
      throw new Error("OAuth response missing refresh_token");
    }
    if (
      (typeof data.expires_in !== "number" &&
        typeof data.expires_in !== "string") ||
      !data.expires_in || isNaN(Number(data.expires_in))
    ) {
      throw new Error("OAuth response missing or invalid expires_in value");
    }

    const expiresIn = Number(data.expires_in);
    const expiresAt = new Date(Date.now() + expiresIn * 1000);

    if (data.access_token.length < 10) {
      throw new Error("Received invalid access_token format");
    }
    if (data.refresh_token.length < 10) {
      throw new Error("Received invalid refresh_token format");
    }

    return {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expires_at: expiresAt,
      token_type: typeof data.token_type === "string"
        ? data.token_type
        : "Bearer",
      scope: typeof data.scope === "string" ? data.scope : INOREADER_OAUTH_SCOPE,
    };
  }
}
