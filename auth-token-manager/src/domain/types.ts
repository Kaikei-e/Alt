/**
 * Domain type definitions for auth-token-manager
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
  redirect_uri: string;
}

export interface AuthenticationResult {
  success: boolean;
  tokens?: TokenResponse;
  error?: string;
  metadata?: {
    duration: number;
    method: string;
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

export interface LoggerConfig {
  level: string;
  include_timestamp: boolean;
  include_stack_trace: boolean;
}

export interface ConfigOptions {
  token_storage_path: string;
  retry: RetryConfig;
  network: NetworkConfig;
  logger: LoggerConfig;
}

export type HealthStatus = "healthy" | "degraded" | "unhealthy";
export type AlertLevel = "info" | "warning" | "critical";

export interface HealthChecks {
  config_valid: boolean;
  environment_ready: boolean;
  storage_ready: boolean;
  refresh_token_available: boolean;
  token_expiry_status: boolean;
}

export interface MonitoringData {
  timestamp: string;
  alert_level: AlertLevel;
  alerts: string[];
  token_status: {
    has_access_token: boolean;
    has_refresh_token: boolean;
    expires_at: string | null;
    updated_at: string | null;
    time_until_expiry_hours: number | null;
    time_since_update_hours: number | null;
    needs_immediate_refresh: boolean;
    needs_refresh_soon: boolean;
  };
  system_status: {
    secret_exists: boolean;
    configuration_valid: boolean;
  };
}
