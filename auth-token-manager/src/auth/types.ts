/**
 * Type definitions for auth-token-manager
 */

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  expires_at: Date;
  token_type?: string;
  scope?: string;
}

export interface InoreaderCredentials {
  client_id: string;
  client_secret: string;
  redirect_uri: string; // Callback URI for initial OAuth2 authorization flow
}

// BrowserConfig removed - browser automation disabled

export interface AuthenticationResult {
  success: boolean;
  tokens?: TokenResponse;
  error?: string;
  metadata?: {
    duration: number;
    method: string; // refresh_token
    session_id: string;
  };
}

export interface RetryConfig {
  max_attempts: number;
  base_delay: number;
  max_delay: number;
  backoff_factor: number;
}

export interface NetworkConfig {
  http_timeout: number;
  connectivity_check: boolean;
  connectivity_timeout: number;
}

export interface SecretData {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  updated_at: string;
  token_type?: string;
  scope?: string;
}

export interface SecretManager {
  updateTokenSecret(tokens: TokenResponse): Promise<void>;
  getTokenSecret(): Promise<SecretData | null>;
  checkSecretExists(): Promise<boolean>;
}

export type K8sSecretData = SecretData;
