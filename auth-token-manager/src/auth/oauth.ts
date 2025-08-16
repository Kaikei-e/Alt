/**
 * Inoreader OAuth token management - Refresh Token Only
 * Browser automation removed - OAuth 2.0 compliant design
 */

import type {
  InoreaderCredentials,
  TokenResponse,
  AuthenticationResult,
  NetworkConfig,
  RetryConfig,
} from "./types.ts";

import { logger } from "../utils/logger.ts";

export class InoreaderTokenManager {
  constructor(
    private credentials: InoreaderCredentials,
    private networkConfig: NetworkConfig = {
      http_timeout: 30000,
      connectivity_check: true,
      connectivity_timeout: 10000,
    },
    private retryConfig: RetryConfig = {
      max_attempts: 3,
      base_delay: 1000,
      max_delay: 30000,
      backoff_factor: 2,
    },
    private kubernetesNamespace: string = "alt-processing",
    private secretName: string = "inoreader-tokens",
  ) {}

  /**
   * Initialize token manager - no browser required
   */
  async initialize(): Promise<void> {
    logger.info("Initializing token manager (refresh-token-only mode)");
    
    // Network connectivity check if enabled
    if (this.networkConfig.connectivity_check) {
      await this.checkNetworkConnectivity();
    }
    
    logger.info("Token manager initialized successfully");
  }

  /**
   * Refresh access token using existing refresh token
   * Browser automation removed - OAuth 2.0 compliant design
   */
  async refreshAccessToken(): Promise<AuthenticationResult> {
    const startTime = Date.now();

    try {
      logger.info("Starting token refresh (refresh-token-only mode)");

      // Network connectivity check
      if (this.networkConfig.connectivity_check) {
        logger.info("Checking network connectivity");
        await this.checkNetworkConnectivity();
        logger.info("Network connectivity verified");
      }

      // Execute refresh token flow with retry logic
      return await this.retryOperation(async () => {
        const refreshResult = await this.executeRefreshTokenFlow();
        
        const duration = Date.now() - startTime;
        logger.info("Token refresh completed successfully", {
          duration_ms: duration,
        });

        return {
          success: true,
          tokens: refreshResult,
          metadata: {
            duration,
            method: "refresh_token",
            session_id: crypto.randomUUID(),
          },
        };
      }, "refresh token flow");

    } catch (error) {
      const duration = Date.now() - startTime;
      const errorMessage = error instanceof Error ? error.message : String(error);

      logger.error("Token refresh failed", { 
        duration_ms: duration,
        error: errorMessage,
      });

      return {
        success: false,
        error: errorMessage,
        metadata: {
          duration,
          method: "refresh_token",
          session_id: crypto.randomUUID(),
        },
      };
    }
  }

  /**
   * Execute refresh token flow to get new access token
   */
  private async executeRefreshTokenFlow(): Promise<TokenResponse> {
    // Import K8s secret manager to get existing refresh token
    const { K8sSecretManager } = await import(
      "../k8s/secret-manager-simple.ts"
    );
    const secretManager = new K8sSecretManager(
      this.kubernetesNamespace,
      this.secretName,
    );

    // Get existing token data
    const existingTokenData = await secretManager.getTokenSecret();
    if (!existingTokenData || !existingTokenData.refresh_token) {
      throw new Error("No existing refresh token found. Manual OAuth setup required.");
    }

    // Validate refresh token before attempting refresh
    if (existingTokenData.refresh_token.length < 10) {
      throw new Error("Invalid refresh token format. Manual OAuth setup required.");
    }

    // Check if current access token is still valid (within 5 minutes of expiry)
    if (existingTokenData.expires_at) {
      const expiresAt = new Date(existingTokenData.expires_at);
      const now = new Date();
      const timeUntilExpiry = expiresAt.getTime() - now.getTime();
      const fiveMinutes = 5 * 60 * 1000;
      
      if (timeUntilExpiry > fiveMinutes) {
        logger.info("Current access token is still valid", {
          expires_at: expiresAt.toISOString(),
          time_until_expiry_minutes: Math.round(timeUntilExpiry / 1000 / 60),
        });
        return {
          access_token: existingTokenData.access_token,
          refresh_token: existingTokenData.refresh_token,
          expires_at: expiresAt,
          token_type: "Bearer",
          scope: "read write",
        };
      }
    }

    logger.info("Found existing refresh token, attempting refresh", {
      refresh_token_length: existingTokenData.refresh_token.length,
    });

    // Use refresh token to get new access token
    // Refresh access token (include empty scope per Inoreader OAuth2 spec)
    const response = await this.fetchWithTimeout(
      "https://www.inoreader.com/oauth2/token",
      {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded",
          "User-Agent": "Auth-Token-Manager/2.0.0",
        },
        body: new URLSearchParams({
          grant_type: "refresh_token",
          client_id: this.credentials.client_id,
          client_secret: this.credentials.client_secret,
          refresh_token: existingTokenData.refresh_token,
          scope: "",
        }),
      },
    );

    if (!response.ok) {
      const errorText = await response.text();
      let errorData: any = {};
      
      try {
        errorData = JSON.parse(errorText);
      } catch {
        // Keep errorText if not valid JSON
      }
      
      const errorCode = errorData.error || 'unknown_error';
      const errorDescription = errorData.error_description || errorText;
      
      logger.error("Refresh token API call failed", {
        status: response.status,
        error_code: errorCode,
        error_description: errorDescription,
        response_headers: Object.fromEntries(response.headers.entries()),
      });

      // Categorize errors for better handling
      switch (errorCode) {
        case 'invalid_grant':
          throw new Error(`Invalid or expired refresh token. Manual OAuth setup required. Details: ${errorDescription}`);
        case 'invalid_client':
          throw new Error(`Invalid client credentials. Check CLIENT_ID and CLIENT_SECRET. Details: ${errorDescription}`);
        case 'unsupported_grant_type':
          throw new Error(`Unsupported grant type. OAuth configuration error. Details: ${errorDescription}`);
        case 'invalid_request':
          throw new Error(`Invalid OAuth request format. Details: ${errorDescription}`);
        default:
          throw new Error(`OAuth refresh failed [${errorCode}]: ${errorDescription} (Status: ${response.status})`);
      }
    }

    const data = await response.json();
    
    // Validate response data
    if (!data.access_token) {
      throw new Error("OAuth response missing access_token");
    }
    if (!data.refresh_token) {
      throw new Error("OAuth response missing refresh_token");
    }
    if (!data.expires_in || isNaN(Number(data.expires_in))) {
      throw new Error("OAuth response missing or invalid expires_in value");
    }

    const expiresIn = Number(data.expires_in);
    const expiresAt = new Date(Date.now() + expiresIn * 1000);

    // Validate token formats
    if (data.access_token.length < 10) {
      throw new Error("Received invalid access_token format");
    }
    if (data.refresh_token.length < 10) {
      throw new Error("Received invalid refresh_token format");
    }

    const tokens: TokenResponse = {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expires_at: expiresAt,
      token_type: data.token_type || "Bearer",
      scope: data.scope || "read write",
    };

    logger.info("Refresh token flow successful", {
      expires_at: expiresAt.toISOString(),
      expires_in_seconds: expiresIn,
      expires_in_hours: Math.round(expiresIn / 3600 * 10) / 10,
      scope: tokens.scope,
      token_type: tokens.token_type,
      access_token_length: data.access_token.length,
      refresh_token_length: data.refresh_token.length,
    });

    return tokens;
  }

  /**
   * Network request with timeout and proxy support
   */
  private async fetchWithTimeout(
    url: string,
    options: RequestInit = {},
  ): Promise<Response> {
    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      this.networkConfig.http_timeout,
    );

    try {
      const proxyUrl = Deno.env.get("HTTPS_PROXY") || Deno.env.get("HTTP_PROXY");
      const fallbackToDirect = Deno.env.get("NETWORK_FALLBACK_TO_DIRECT") === "true";
      
      let fetchOptions: RequestInit = {
        ...options,
        signal: controller.signal,
      };

      // First attempt: Try with proxy if configured
      if (proxyUrl) {
        try {
          const proxyHost = new URL(proxyUrl).host;
          const targetHost = new URL(url).host;
          logger.info("Attempting proxy connection", {
            proxy_host: proxyHost,
            target_host: targetHost,
            proxy_url: proxyUrl,
          });

          // Test proxy connectivity first with shorter timeout
          const proxyTestController = new AbortController();
          const proxyTestTimeout = setTimeout(
            () => proxyTestController.abort(),
            10000, // 10 second proxy test timeout
          );

          try {
            await fetch(url, {
              ...fetchOptions,
              signal: proxyTestController.signal,
            });
            clearTimeout(proxyTestTimeout);
            
            // Proxy works, use normal timeout for actual request
            const response = await fetch(url, fetchOptions);
            logger.info("Proxy connection successful");
            return response;
            
          } catch (proxyError) {
            clearTimeout(proxyTestTimeout);
            logger.warn("Proxy connection failed", {
              error: proxyError instanceof Error ? proxyError.message : String(proxyError),
              proxy_url: proxyUrl,
              target_url: url,
            });
            
            if (!fallbackToDirect) {
              logger.error("Proxy connection failed and direct fallback disabled", {
                proxy_url: proxyUrl,
                target_url: url,
                error: proxyError instanceof Error ? proxyError.message : String(proxyError),
              });
              throw new Error(`Proxy connection required but failed: ${proxyError instanceof Error ? proxyError.message : String(proxyError)}`);
            }
            
            logger.info("Attempting direct connection fallback");
          }
        } catch (proxySetupError) {
          logger.warn("Proxy setup failed", {
            error: proxySetupError instanceof Error ? proxySetupError.message : String(proxySetupError),
          });
          
          if (!fallbackToDirect) {
            logger.error("Proxy setup failed and direct fallback disabled", {
              proxy_url: proxyUrl,
              target_url: url,
              error: proxySetupError instanceof Error ? proxySetupError.message : String(proxySetupError),
            });
            throw new Error(`Proxy setup required but failed: ${proxySetupError instanceof Error ? proxySetupError.message : String(proxySetupError)}`);
          }
        }
      }

      // Second attempt: Direct connection (either no proxy configured or fallback enabled)
      logger.info("Using direct connection", { target_url: url });
      
      // Remove proxy-related environment variables temporarily for direct connection
      const originalHttpProxy = Deno.env.get("HTTP_PROXY");
      const originalHttpsProxy = Deno.env.get("HTTPS_PROXY");
      
      if (fallbackToDirect && (originalHttpProxy || originalHttpsProxy)) {
        logger.info("Temporarily disabling proxy for direct connection");
        if (originalHttpProxy) Deno.env.delete("HTTP_PROXY");
        if (originalHttpsProxy) Deno.env.delete("HTTPS_PROXY");
      }

      try {
        const directFetchOptions: RequestInit = {
          ...options,
          signal: controller.signal,
        };
        
        const response = await fetch(url, directFetchOptions);
        logger.info("Direct connection successful");
        return response;
      } finally {
        // Restore proxy environment variables
        if (fallbackToDirect) {
          if (originalHttpProxy) Deno.env.set("HTTP_PROXY", originalHttpProxy);
          if (originalHttpsProxy) Deno.env.set("HTTPS_PROXY", originalHttpsProxy);
        }
      }
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(
          `HTTP request timed out after ${this.networkConfig.http_timeout}ms: ${url}`,
        );
      }
      logger.error("All connection attempts failed", {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Check network connectivity to Inoreader
   */
  private async checkNetworkConnectivity(): Promise<void> {
    try {
      const proxyUrl = Deno.env.get("HTTPS_PROXY") || Deno.env.get("HTTP_PROXY");
      if (proxyUrl) {
        const proxyHost = new URL(proxyUrl).host;
        logger.info("Connectivity check via proxy", { proxy_host: proxyHost });
      }

      const response = await this.fetchWithTimeout(
        "https://www.inoreader.com",
        {
          method: "HEAD",
        },
      );

      if (!response.ok && response.status >= 500) {
        throw new Error(`Inoreader server error: ${response.status}`);
      }

      logger.info("Network connectivity verified", {
        status: response.status,
      });
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      throw new Error(`Network connectivity check failed: ${errorMessage}`);
    }
  }

  /**
   * Retry operation with exponential backoff
   */
  private async retryOperation<T>(
    operation: () => Promise<T>,
    operationName: string,
  ): Promise<T> {
    let lastError: Error;

    for (let attempt = 1; attempt <= this.retryConfig.max_attempts; attempt++) {
      try {
        logger.info(
          `Attempt ${attempt}/${this.retryConfig.max_attempts}`,
          { operation: operationName },
        );
        return await operation();
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));

        if (attempt === this.retryConfig.max_attempts) {
          logger.error(`All ${this.retryConfig.max_attempts} attempts failed`, {
            operation: operationName,
          });
          throw lastError;
        }

        const delay = Math.min(
          this.retryConfig.base_delay *
            Math.pow(this.retryConfig.backoff_factor, attempt - 1),
          this.retryConfig.max_delay,
        );

        logger.warn(`Attempt ${attempt} failed`, {
          operation: operationName,
          next_delay_ms: delay,
        });
        await this.sleep(delay);
      }
    }

    throw lastError!;
  }

  /**
   * Sleep for specified milliseconds
   */
  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
