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
  username: string;
  password: string;
  client_id: string;
  client_secret: string;
  redirect_uri: string;
}

export interface BrowserConfig {
  headless: boolean;
  width: number;
  height: number;
  args: string[];
  viewport: { width: number; height: number };
  user_agent?: string;
  locale?: string;
  timezone?: string;
  timeouts: {
    navigation: number;
    element_wait: number;
    authorization_code: number;
    consent_form: number;
  };
}

export interface AuthenticationResult {
  success: boolean;
  tokens?: TokenResponse;
  error?: string;
  metadata?: {
    duration: number;
    user_agent: string;
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

export class BrowserError extends Error {
  constructor(
    message: string,
    public readonly context?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'BrowserError';
  }
}

export class K8sError extends Error {
  constructor(
    message: string,
    public readonly context?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'K8sError';
  }
}