/**
 * TokenClient port - interface for OAuth token operations
 */
import type { TokenResponse } from "../domain/types.ts";

export interface TokenClient {
  refreshToken(refreshToken: string): Promise<TokenResponse>;
  exchangeCode(code: string): Promise<TokenResponse>;
}
