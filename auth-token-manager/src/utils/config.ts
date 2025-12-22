/**
 * Configuration management for auth-token-manager
 */

import type { InoreaderCredentials, RetryConfig, NetworkConfig } from '../auth/types.ts';

export interface LoggerConfig {
  level: string;
  include_timestamp: boolean;
  include_stack_trace: boolean;
}

export interface ConfigOptions {
  token_storage_type: 'kubernetes_secret' | 'file';
  token_storage_path: string;
  kubernetes_namespace: string;
  secret_name: string;
  retry: RetryConfig;
  network: NetworkConfig;
  logger: LoggerConfig;
}

export interface KubernetesConfig {
  namespace: string;
  secretName: string;
}

class ConfigManager {
  private static instance: ConfigManager;

  static getInstance(): ConfigManager {
    if (!this.instance) {
      this.instance = new ConfigManager();
    }
    return this.instance;
  }

  async loadConfig(): Promise<ConfigOptions> {
    const configOptions: ConfigOptions = {
      token_storage_type: (Deno.env.get('TOKEN_STORAGE_TYPE') as 'kubernetes_secret' | 'file') || 'kubernetes_secret',
      token_storage_path: Deno.env.get('TOKEN_STORAGE_PATH') || '/app/secrets/oauth2_token.env',
      kubernetes_namespace: Deno.env.get('KUBERNETES_NAMESPACE') || 'alt-processing',
      secret_name: Deno.env.get('SECRET_NAME') || 'pre-processor-sidecar-oauth2-token',
      retry: {
        max_attempts: parseInt(Deno.env.get('RETRY_MAX_ATTEMPTS') || '3'),
        base_delay: parseInt(Deno.env.get('RETRY_BASE_DELAY') || '1000'),
        max_delay: parseInt(Deno.env.get('RETRY_MAX_DELAY') || '30000'),
        backoff_factor: parseFloat(Deno.env.get('RETRY_BACKOFF_FACTOR') || '2'),
      },
      network: {
        http_timeout: parseInt(Deno.env.get('HTTP_TIMEOUT') || '30000'), // 30s
        connectivity_check: Deno.env.get('CONNECTIVITY_CHECK') !== 'false',
        connectivity_timeout: parseInt(Deno.env.get('CONNECTIVITY_TIMEOUT') || '10000'), // 10s
      },
      logger: {
        level: (Deno.env.get('LOG_LEVEL') || 'INFO').toUpperCase(),
        include_timestamp: Deno.env.get('LOG_INCLUDE_TIMESTAMP') !== 'false',
        include_stack_trace: Deno.env.get('LOG_INCLUDE_STACK_TRACE') !== 'false',
      },
    };

    console.log(`[DEBUG] Config loaded. Log level: ${configOptions.logger.level}`);
    return configOptions;
  }

  validateConfig(): boolean {
    // Required for refresh token OAuth flow
    const required = [
      'INOREADER_CLIENT_ID',
      'INOREADER_CLIENT_SECRET',
    ];

    // Check required environment variables
    for (const env of required) {
      const value = this.getEnvOrFile(env);
      if (!value) {
        console.error(`Missing required environment variable: ${env} (or ${env}_FILE)`);
        return false;
      }

      // Enhanced validation: Check for dummy/placeholder values
      if (value === 'demo-client-id' || value === 'demo-client-secret' || value === 'placeholder') {
        console.error(`Invalid placeholder value for ${env}: ${value}`);
        return false;
      }

      // Check minimum length for security
      if (value.length < 5) {
        console.error(`Invalid ${env}: too short (minimum 5 characters required)`);
        return false;
      }
    }

    // Check storage configuration
    const type = Deno.env.get('TOKEN_STORAGE_TYPE');
    if (type && type !== 'kubernetes_secret' && type !== 'file') {
      console.error(`Invalid TOKEN_STORAGE_TYPE: ${type}. Must be 'kubernetes_secret' or 'file'`);
      return false;
    }

    if (type === 'file' && !Deno.env.get('TOKEN_STORAGE_PATH')) {
      console.error('TOKEN_STORAGE_PATH is required when TOKEN_STORAGE_TYPE is "file"');
      // We have a default, but explicit check is good practice if default fails
    }

    // Enhanced logging with configuration status
    console.log('✅ Configuration validation successful');
    console.log('ℹ️ Using refresh token flow only (browser automation disabled)');
    console.log(`ℹ️ Storage mode: ${type || 'kubernetes_secret'}`);

    // Log configuration summary (without sensitive data)
    const clientId = this.getEnvOrFile('INOREADER_CLIENT_ID');
    console.log(`ℹ️ Client ID configured: ${clientId ? clientId.substring(0, 4) + '...' + clientId.substring(clientId.length - 10) : '[NOT_SET]'}`);

    return true;
  }

  getInoreaderCredentials(): InoreaderCredentials {
    return {
      client_id: this.getEnvOrFile('INOREADER_CLIENT_ID')!,
      client_secret: this.getEnvOrFile('INOREADER_CLIENT_SECRET')!,
      redirect_uri: Deno.env.get('INOREADER_REDIRECT_URI') || 'http://localhost:8080/callback',
    };
  }

  getKubernetesConfig(): KubernetesConfig {
    return {
      namespace: Deno.env.get('KUBERNETES_NAMESPACE') || 'alt-processing',
      secretName: Deno.env.get('SECRET_NAME') || 'pre-processor-sidecar-oauth2-token',
    };
  }

  isProductionMode(): boolean {
    const env = Deno.env.get('NODE_ENV') || Deno.env.get('DENO_ENV') || 'development';
    return env === 'production';
  }

  private getEnvOrFile(key: string): string | undefined {
    // Try environment variable first
    const val = Deno.env.get(key);
    if (val) return val;

    // Try file path from _FILE environment variable
    const filePath = Deno.env.get(`${key}_FILE`);
    if (filePath) {
      try {
        // Synchronous read is acceptable during startup/config loading
        return Deno.readTextFileSync(filePath).trim();
      } catch (error) {
        console.warn(`Failed to read secret file ${filePath} for ${key}:`, error);
      }
    }

    return undefined;
  }
}

export const config = ConfigManager.getInstance();
