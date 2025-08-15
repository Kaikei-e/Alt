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
      "alt-processing",
      "pre-processor-sidecar-oauth2-token",
    );

    // Get existing token data
    const existingTokenData = await secretManager.getTokenSecret();
    if (!existingTokenData || !existingTokenData.refresh_token) {
      throw new Error("No existing refresh token found. Manual OAuth setup required.");
    }

    logger.info("Found existing refresh token, attempting refresh");

    // Use refresh token to get new access token
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
        }),
      },
    );

    if (!response.ok) {
      const errorText = await response.text();
      logger.error("Refresh token API call failed", {
        status: response.status,
        error: errorText,
      });
      throw new Error(`Refresh token failed: ${response.status} ${errorText}`);
    }

    const data = await response.json();
    const expiresAt = new Date(Date.now() + data.expires_in * 1000);

    const tokens: TokenResponse = {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expires_at: expiresAt,
      token_type: data.token_type || "Bearer",
      scope: data.scope || "read write",
    };

    logger.info("Refresh token flow successful", {
      expires_at: expiresAt.toISOString(),
      scope: tokens.scope,
    });

    return tokens;
  }

  private async handleLoginForm(): Promise<void> {
    if (!this.page) throw new Error("Page not initialized");

    // Wait for login form with multiple selector fallbacks
    await this.page.waitForSelector("#username, #email, input[name='username'], input[name='email']", {
      timeout: this.browserConfig.timeouts.element_wait,
    });

    // Fill credentials with robust selectors
    await this.page.fill("#username, #email, input[name='username'], input[name='email']", this.credentials.username);
    await this.page.fill("#passwd, #password, input[name='password'], input[type='password']", this.credentials.password);

    // Submit form
    await this.page.click('input[type="submit"]');

    // Wait for navigation
    await this.page.waitForNavigation({
      waitUntil: "networkidle",
      timeout: this.browserConfig.timeouts.navigation,
    });
  }

  private async handleAuthorizationConsent(): Promise<void> {
    if (!this.page) throw new Error("Page not initialized");

    try {
      // Look for authorization consent button
      const consentButton = await this.page.waitForSelector(
        'input[value="Allow"], button:has-text("Allow"), input[value="Authorize"], button:has-text("Authorize")',
        { timeout: this.browserConfig.timeouts.consent_form },
      );

      if (consentButton) {
          logger.info("Found consent button, clicking");
        await consentButton.click();
        await this.page.waitForNavigation({
          waitUntil: "networkidle",
          timeout: this.browserConfig.timeouts.navigation,
        });
      }
    } catch (error) {
        logger.info("No consent page found, proceeding");
    }
  }

  private async captureAuthorizationCode(): Promise<string> {
    if (!this.page) throw new Error("Page not initialized");

    // Check if using OOB (out-of-band) flow
    if (this.credentials.redirect_uri === "urn:ietf:wg:oauth:2.0:oob") {
      // For OOB flow, look for the authorization code in the page content
      try {
        // Wait for the authorization code to appear on the page
        await this.page.waitForSelector("input[readonly], code, .code", {
          timeout: this.browserConfig.timeouts.authorization_code,
        });

        // Try different selectors to find the authorization code
        const codeElement =
          (await this.page.$("input[readonly]")) ||
          (await this.page.$("code")) ||
          (await this.page.$(".code")) ||
          (await this.page.$("[data-code]"));

        if (codeElement) {
          const code =
            (await codeElement.textContent()) ||
            (await codeElement.getAttribute("value"));
          if (code && code.trim()) {
              logger.info("Found authorization code via OOB flow");
            return code.trim();
          }
        }

        // Fallback: look for code in page text
        const pageContent = (await this.page.textContent("body")) || "";
        const codeMatch = pageContent.match(/\b[A-Za-z0-9]{20,}\b/);
        if (codeMatch) {
            logger.info("Found authorization code in page content");
          return codeMatch[0];
        }

        throw new Error("Authorization code not found in OOB response");
      } catch (error) {
          logger.error("OOB code capture failed, trying URL-based capture");
        // Fallback to URL-based capture
      }
    }

    // Standard callback URL flow
    await this.page.waitForURL(/callback|code=/, {
      timeout: this.browserConfig.timeouts.authorization_code,
    });

    const url = this.page.url();
    const urlObj = new URL(url);
    const code = urlObj.searchParams.get("code");

    if (!code) {
      throw new Error("Authorization code not found in callback URL");
    }

    return code;
  }

  private async exchangeCodeForTokens(code: string): Promise<TokenResponse> {
    const tokenUrl = "https://www.inoreader.com/oauth2/token";

    const body = new URLSearchParams({
      grant_type: "authorization_code",
      client_id: this.credentials.client_id,
      client_secret: this.credentials.client_secret,
      code: code,
      redirect_uri: this.credentials.redirect_uri,
    });

    const response = await this.fetchWithTimeout(tokenUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "User-Agent":
          this.browserConfig.user_agent || "Auth-Token-Manager/2.0.0",
      },
      body: body.toString(),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Token exchange failed: ${response.status} ${errorText}`);
    }

    const data = await response.json();

    const expiresAt = new Date(Date.now() + data.expires_in * 1000);

    return {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expires_at: expiresAt,
      token_type: data.token_type,
      scope: data.scope,
    };
  }

  private async tryRefreshTokenFlow(): Promise<AuthenticationResult> {
    try {
      // Import K8s secret manager to get existing refresh token
      const { K8sSecretManager } = await import(
        "../k8s/secret-manager-simple.ts"
      );
      const secretManager = new K8sSecretManager(
        "alt-processing",
        "pre-processor-sidecar-oauth2-token",
      );

      // Try to get existing token data
      const existingTokenData = await secretManager.getTokenSecret();
      if (!existingTokenData || !existingTokenData.refresh_token) {
        throw new Error("No existing refresh token found");
      }

        logger.info("Found existing refresh token, attempting refresh");

      // Use refresh token to get new access token
      const response = await this.fetchWithTimeout(
        "https://www.inoreader.com/oauth2/token",
        {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
            "User-Agent":
              this.browserConfig.user_agent || "Auth-Token-Manager/2.0.0",
          },
          body: new URLSearchParams({
            grant_type: "refresh_token",
            client_id: this.credentials.client_id,
            client_secret: this.credentials.client_secret,
            refresh_token: existingTokenData.refresh_token,
          }),
        },
      );

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(
          `Refresh token failed: ${response.status} ${errorText}`,
        );
      }

      const data = await response.json();
      const expiresAt = new Date(Date.now() + data.expires_in * 1000);

      const tokens: TokenResponse = {
        access_token: data.access_token,
        refresh_token: data.refresh_token,
        expires_at: expiresAt,
        token_type: data.token_type || "Bearer",
        scope: data.scope || "read write",
      };

        logger.info("Refresh token flow successful");

      return {
        success: true,
        tokens,
        metadata: {
          duration: 0,
          user_agent: this.browserConfig.user_agent || "unknown",
          session_id: crypto.randomUUID(),
          method: "refresh_token",
        },
      };
    } catch (error) {
      // Do not expose detailed error information in logs
        logger.error("Refresh token flow failed");
      throw error;
    }
  }

  async cleanup(): Promise<void> {
    try {
      if (this.page) {
        await this.page.close();
        this.page = null;
      }

      if (this.context) {
        await this.context.close();
        this.context = null;
      }

      if (this.browser) {
        await this.browser.close();
        this.browser = null;
      }

        logger.info("Browser cleanup completed");
      } catch {
        logger.warn("Error during browser cleanup");
      }
  }

  // Utility methods for network operations and retry logic

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
      // 恒久対応: プロキシ接続とフォールバック戦略
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

  private async checkNetworkConnectivity(): Promise<void> {
    try {
      // Check if we're using a proxy
        const proxyUrl =
          Deno.env.get("HTTPS_PROXY") || Deno.env.get("HTTP_PROXY");
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
      const errorMessage =
        error instanceof Error ? error.message : String(error);
      throw new Error(`Network connectivity check failed: ${errorMessage}`);
    }
  }

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

        // Reset browser state for next attempt if it's a browser operation
        if (
          operationName.includes("browser") ||
          operationName.includes("OAuth")
        ) {
          try {
            await this.resetBrowserState();
          } catch {
            logger.warn("Browser state reset failed", { operation: operationName });
          }
        }
      }
    }

    throw lastError!;
  }

  private async resetBrowserState(): Promise<void> {
    if (this.page && !this.page.isClosed()) {
      try {
        // Navigate to blank page to reset state
        await this.page.goto("about:blank", { timeout: 10000 });
        } catch {
          logger.warn("Failed to reset page state", { operation: "resetBrowserState" });
        }
      }
    }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
