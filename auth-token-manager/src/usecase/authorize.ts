/**
 * AuthorizeUsecase - Initial OAuth2 authorization flow
 */

import type { TokenClient } from "../port/token_client.ts";
import type { SecretManager } from "../port/secret_manager.ts";
import type { InoreaderCredentials } from "../domain/types.ts";
import { logger } from "../infra/logger.ts";

export class AuthorizeUsecase {
  constructor(
    private tokenClient: TokenClient,
    private secretManager: SecretManager,
    private credentials: InoreaderCredentials,
  ) {}

  buildAuthorizationUrl(): { url: string; state: string } {
    const authUrl = new URL("https://www.inoreader.com/oauth2/auth");
    authUrl.searchParams.set("client_id", this.credentials.client_id);
    authUrl.searchParams.set("redirect_uri", this.credentials.redirect_uri);
    authUrl.searchParams.set("response_type", "code");
    authUrl.searchParams.set("scope", "read");

    const state = crypto.randomUUID();
    authUrl.searchParams.set("state", state);

    return { url: authUrl.toString(), state };
  }

  async exchangeCodeAndStore(code: string): Promise<void> {
    logger.info("Exchanging authorization code for tokens");

    const tokens = await this.tokenClient.exchangeCode(code);
    await this.secretManager.updateTokenSecret(tokens);

    logger.info("Authorization completed - tokens stored");
  }
}
