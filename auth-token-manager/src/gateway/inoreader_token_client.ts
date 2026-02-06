/**
 * Inoreader OAuth token client gateway
 */

import type { TokenClient } from "../port/token_client.ts";
import type { HttpClient } from "../port/http_client.ts";
import type { InoreaderCredentials, TokenResponse } from "../domain/types.ts";
import { logger } from "../infra/logger.ts";

const INOREADER_TOKEN_URL = "https://www.inoreader.com/oauth2/token";

export class InoreaderTokenClient implements TokenClient {
  constructor(
    private credentials: InoreaderCredentials,
    private httpClient: HttpClient,
  ) {}

  async refreshToken(refreshToken: string): Promise<TokenResponse> {
    logger.info("Requesting token refresh from Inoreader");

    const response = await this.httpClient.fetch(INOREADER_TOKEN_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "User-Agent": "Auth-Token-Manager/2.0.0",
      },
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
          throw new Error(
            `Invalid or expired refresh token. Manual OAuth setup required. Details: ${errorDescription}`,
          );
        case "invalid_client":
          throw new Error(
            `Invalid client credentials. Check CLIENT_ID and CLIENT_SECRET. Details: ${errorDescription}`,
          );
        case "unsupported_grant_type":
          throw new Error(
            `Unsupported grant type. OAuth configuration error. Details: ${errorDescription}`,
          );
        case "invalid_request":
          throw new Error(
            `Invalid OAuth request format. Details: ${errorDescription}`,
          );
        default:
          throw new Error(
            `OAuth refresh failed [${errorCode}]: ${errorDescription} (Status: ${response.status})`,
          );
      }
    }

    const data = await response.json();
    return this.validateAndBuildTokenResponse(data);
  }

  async exchangeCode(code: string): Promise<TokenResponse> {
    logger.info("Exchanging authorization code for tokens");

    const response = await this.httpClient.fetch(INOREADER_TOKEN_URL, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
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

    const data = await response.json();
    return this.validateAndBuildTokenResponse(data);
  }

  // deno-lint-ignore no-explicit-any
  private validateAndBuildTokenResponse(data: any): TokenResponse {
    if (!data.access_token) {
      throw new Error("OAuth response missing access_token");
    }
    if (!data.refresh_token) {
      throw new Error("OAuth response missing refresh_token");
    }
    if (!data.expires_in || isNaN(Number(data.expires_in))) {
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
      token_type: data.token_type || "Bearer",
      scope: data.scope || "read write",
    };
  }
}
