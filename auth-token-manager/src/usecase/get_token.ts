/**
 * GetTokenUsecase - Retrieves current token secret data from storage
 */

import type { SecretManager } from "../port/secret_manager.ts";
import type { SecretData } from "../domain/types.ts";
import { logger } from "../infra/logger.ts";

export class GetTokenUsecase {
  constructor(private secretManager: SecretManager) {
    if (!secretManager) {
      throw new Error(
        "GetTokenUsecase: secretManager is required and must be wired at composition root",
      );
    }
  }

  async execute(): Promise<SecretData | null> {
    logger.debug("Retrieving token secret from storage");
    return await this.secretManager.getTokenSecret();
  }

  async getPublicToken(): Promise<Omit<SecretData, "refresh_token"> | null> {
    const tokenData = await this.execute();
    if (!tokenData) {
      return null;
    }
    // Explicit allowlist of returned fields: access_token, expires_at, updated_at, token_type, scope.
    // Defense-in-depth: new SecretData fields or internal storage properties never leak.
    const publicToken: Omit<SecretData, "refresh_token"> = {
      access_token: tokenData.access_token,
      expires_at: tokenData.expires_at,
      updated_at: tokenData.updated_at,
    };
    if (tokenData.token_type !== undefined) {
      publicToken.token_type = tokenData.token_type;
    }
    if (tokenData.scope !== undefined) {
      publicToken.scope = tokenData.scope;
    }
    return publicToken;
  }
}
