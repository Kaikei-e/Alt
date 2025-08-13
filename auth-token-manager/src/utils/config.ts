/**
 * Configuration management for auth-token-manager
 */

import type { BrowserConfig, InoreaderCredentials, RetryConfig, NetworkConfig } from '../auth/types.ts';

export interface LoggerConfig {
  level: string;
  include_timestamp: boolean;
  include_stack_trace: boolean;
}

export interface ConfigOptions {
  kubernetes_namespace: string;
  secret_name: string;
  browser: BrowserConfig;
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
    return {
      kubernetes_namespace: Deno.env.get('KUBERNETES_NAMESPACE') || 'alt-processing',
      secret_name: Deno.env.get('SECRET_NAME') || 'inoreader-tokens',
      browser: {
        headless: Deno.env.get('BROWSER_HEADLESS') !== 'false',
        width: parseInt(Deno.env.get('BROWSER_WIDTH') || '1920'),
        height: parseInt(Deno.env.get('BROWSER_HEIGHT') || '1080'),
        args: (Deno.env.get('BROWSER_ARGS') || '--no-sandbox,--disable-dev-shm-usage').split(','),
        viewport: {
          width: parseInt(Deno.env.get('BROWSER_WIDTH') || '1920'),
          height: parseInt(Deno.env.get('BROWSER_HEIGHT') || '1080'),
        },
        user_agent: Deno.env.get('BROWSER_USER_AGENT'),
        locale: Deno.env.get('BROWSER_LOCALE') || 'en-US',
        timezone: Deno.env.get('BROWSER_TIMEZONE') || 'UTC',
        timeouts: {
          navigation: parseInt(Deno.env.get('BROWSER_NAVIGATION_TIMEOUT') || '90000'), // 90s
          element_wait: parseInt(Deno.env.get('BROWSER_ELEMENT_TIMEOUT') || '30000'), // 30s
          authorization_code: parseInt(Deno.env.get('BROWSER_AUTH_CODE_TIMEOUT') || '45000'), // 45s
          consent_form: parseInt(Deno.env.get('BROWSER_CONSENT_TIMEOUT') || '15000'), // 15s
        },
      },
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
        level: Deno.env.get('LOG_LEVEL') || 'INFO',
        include_timestamp: Deno.env.get('LOG_INCLUDE_TIMESTAMP') !== 'false',
        include_stack_trace: Deno.env.get('LOG_INCLUDE_STACK_TRACE') !== 'false',
      },
    };
  }

  validateConfig(): boolean {
    // Always required for OAuth API calls
    const alwaysRequired = [
      'INOREADER_CLIENT_ID',
      'INOREADER_CLIENT_SECRET',
    ];

    // Only required for browser automation (not for refresh token flows)
    const browserRequired = [
      'INOREADER_USERNAME',
      'INOREADER_PASSWORD',
    ];

    // Check always required
    for (const env of alwaysRequired) {
      if (!Deno.env.get(env)) {
        console.error(`Missing required environment variable: ${env}`);
        return false;
      }
    }

    // Check if we have browser credentials OR can use refresh tokens
    const hasCredentials = browserRequired.every(env => Deno.env.get(env));
    const canUseRefreshToken = Deno.env.get('INOREADER_CLIENT_ID') && Deno.env.get('INOREADER_CLIENT_SECRET');

    if (!hasCredentials && !canUseRefreshToken) {
      console.error('Missing credentials: Need either USERNAME/PASSWORD for browser automation OR CLIENT_ID/CLIENT_SECRET for refresh token flow');
      return false;
    }

    if (!hasCredentials) {
      console.log('ℹ️ No browser credentials provided, will use refresh token flow only');
    }

    return true;
  }

  getInoreaderCredentials(): InoreaderCredentials {
    return {
      username: Deno.env.get('INOREADER_USERNAME') || '',
      password: Deno.env.get('INOREADER_PASSWORD') || '',
      client_id: Deno.env.get('INOREADER_CLIENT_ID')!,
      client_secret: Deno.env.get('INOREADER_CLIENT_SECRET')!,
      redirect_uri: Deno.env.get('INOREADER_REDIRECT_URI') || 'urn:ietf:wg:oauth:2.0:oob',
    };
  }

  getKubernetesConfig(): KubernetesConfig {
    return {
      namespace: Deno.env.get('KUBERNETES_NAMESPACE') || 'alt-processing',
      secretName: Deno.env.get('SECRET_NAME') || 'inoreader-tokens',
    };
  }

  isProductionMode(): boolean {
    const env = Deno.env.get('NODE_ENV') || Deno.env.get('DENO_ENV') || 'development';
    return env === 'production';
  }
}

export const config = ConfigManager.getInstance();