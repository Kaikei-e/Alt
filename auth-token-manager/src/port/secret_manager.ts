/**
 * SecretManager port - interface for token storage operations
 */
import type { SecretData, TokenResponse } from "../domain/types.ts";

export interface SecretManager {
  updateTokenSecret(tokens: TokenResponse): Promise<void>;
  getTokenSecret(): Promise<SecretData | null>;
  checkSecretExists(): Promise<boolean>;
}
