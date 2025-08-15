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
  redirect_uri: string; // For documentation purposes - not used in refresh-token-only mode
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

export interface K8sSecretData {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  updated_at: string;
}

export class AuthError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly context?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'AuthError';
  }
}

// BrowserError removed - browser automation disabled

export class K8sError extends Error {
  constructor(
    message: string,
    public readonly context?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'K8sError';
  }
}