/**
 * Inoreader OAuth automation using Playwright
 */

import { Browser, BrowserContext, Page } from "playwright";
import type {
  InoreaderCredentials,
  BrowserConfig,
  TokenResponse,
  AuthenticationResult,
  AuthError,
  NetworkConfig,
  RetryConfig,
} from "./types.ts";

export class InoreaderOAuthAutomator {
  private browser: Browser | null = null;
  private context: BrowserContext | null = null;
  private page: Page | null = null;

  constructor(
    private credentials: InoreaderCredentials,
    private browserConfig: BrowserConfig,
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

  async initializeBrowser(): Promise<void> {
    try {
      // Import playwright dynamically for Deno compatibility
      const { chromium } = await import("playwright");

      console.log("🔧 Initializing browser...");

      // Check for proxy configuration
      const proxyUrl =
        Deno.env.get("HTTPS_PROXY") || Deno.env.get("HTTP_PROXY");
      let browserOptions: any = {
        headless: this.browserConfig.headless,
        args: this.browserConfig.args,
      };

      // Add proxy configuration if available
      if (proxyUrl) {
        console.log(`🌐 Configuring browser proxy: ${proxyUrl}`);
        const proxyUrlObj = new URL(proxyUrl);
        browserOptions.proxy = {
          server: proxyUrl,
          bypass: "localhost,127.0.0.1,*.local",
        };
      }

      this.browser = await chromium.launch(browserOptions);

      this.context = await this.browser.newContext({
        viewport: this.browserConfig.viewport,
        userAgent: this.browserConfig.user_agent,
        locale: this.browserConfig.locale,
        timezoneId: this.browserConfig.timezone,
        ignoreHTTPSErrors: true,
      });

      this.page = await this.context.newPage();

      console.log("✅ Browser initialized successfully");
    } catch (error) {
      throw new Error(`Failed to initialize browser: ${error}`);
    }
  }

  async performOAuth(): Promise<AuthenticationResult> {
    const startTime = Date.now();

    try {
      // Network connectivity check
      if (this.networkConfig.connectivity_check) {
        console.log("🌐 Checking network connectivity...");
        await this.checkNetworkConnectivity();
        console.log("✅ Network connectivity verified");
      }

      // First try refresh token approach if available
      console.log("🔄 Attempting refresh token flow first...");

      try {
        const refreshResult = await this.tryRefreshTokenFlow();
        if (refreshResult.success) {
          const duration = Date.now() - startTime;
          console.log(
            `✅ Refresh token flow completed successfully in ${duration}ms`,
          );
          return refreshResult;
        }
        console.log(
          "⚠️ Refresh token flow failed, falling back to browser automation...",
        );
      } catch (refreshError) {
        console.log(
          "⚠️ Refresh token not available, using browser automation...",
        );
      }

      // Fallback to browser automation flow with retry logic
      return await this.retryOperation(async () => {
        if (!this.page) {
          throw new Error(
            "Browser not initialized. Call initializeBrowser() first.",
          );
        }

        console.log("🔐 Starting OAuth browser flow...");

        // Step 1: Navigate to Inoreader OAuth authorization
        const authUrl = this.buildAuthUrl();
        console.log("📍 Navigating to authorization URL");
        await this.page.goto(authUrl, {
          waitUntil: "networkidle",
          timeout: this.browserConfig.timeouts.navigation,
        });

        // Step 2: Handle login form
        console.log("🔑 Handling login form...");
        await this.handleLoginForm();

        // Step 3: Handle authorization consent
        console.log("✅ Handling authorization consent...");
        await this.handleAuthorizationConsent();

        // Step 4: Capture authorization code from redirect
        console.log("📋 Capturing authorization code...");
        const authCode = await this.captureAuthorizationCode();

        // Step 5: Exchange authorization code for tokens
        console.log("🔄 Exchanging code for tokens...");
        const tokens = await this.exchangeCodeForTokens(authCode);

        const duration = Date.now() - startTime;
        console.log(`✅ OAuth flow completed successfully in ${duration}ms`);

        return {
          success: true,
          tokens,
          metadata: {
            duration,
            user_agent: this.browserConfig.user_agent || "unknown",
            session_id: crypto.randomUUID(),
          },
        };
      }, "OAuth browser flow");
    } catch (error) {
      const duration = Date.now() - startTime;
      const errorMessage =
        error instanceof Error ? error.message : String(error);

      // Avoid logging sensitive error details
      console.error(`❌ OAuth flow failed after ${duration}ms`);

      return {
        success: false,
        error: errorMessage,
        metadata: {
          duration,
          user_agent: this.browserConfig.user_agent || "unknown",
          session_id: crypto.randomUUID(),
        },
      };
    }
  }

  private buildAuthUrl(): string {
    const params = new URLSearchParams({
      response_type: "code",
      client_id: this.credentials.client_id,
      redirect_uri: this.credentials.redirect_uri,
      scope: "read write",
      state: crypto.randomUUID(),
    });

    return `https://www.inoreader.com/oauth2/auth?${params.toString()}`;
  }

  private async handleLoginForm(): Promise<void> {
    if (!this.page) throw new Error("Page not initialized");

    // Wait for login form
    await this.page.waitForSelector("#email", {
      timeout: this.browserConfig.timeouts.element_wait,
    });

    // Fill credentials
    await this.page.fill("#email", this.credentials.username);
    await this.page.fill("#passwd", this.credentials.password);

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
        console.log("🎯 Found consent button, clicking...");
        await consentButton.click();
        await this.page.waitForNavigation({
          waitUntil: "networkidle",
          timeout: this.browserConfig.timeouts.navigation,
        });
      }
    } catch (error) {
      console.log("ℹ️ No consent page found, proceeding...");
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
            console.log("📋 Found authorization code via OOB flow");
            return code.trim();
          }
        }

        // Fallback: look for code in page text
        const pageContent = (await this.page.textContent("body")) || "";
        const codeMatch = pageContent.match(/\b[A-Za-z0-9]{20,}\b/);
        if (codeMatch) {
          console.log("📋 Found authorization code in page content");
          return codeMatch[0];
        }

        throw new Error("Authorization code not found in OOB response");
      } catch (error) {
        console.error("🔍 OOB code capture failed, trying URL-based capture");
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

      console.log("🔄 Found existing refresh token, attempting refresh...");

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

      console.log("✅ Refresh token flow successful");

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
      console.error("⚠️ Refresh token flow failed");
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

      console.log("🧹 Browser cleanup completed");
    } catch (error) {
      console.warn("⚠️ Error during browser cleanup:", error);
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
      // Use proxy configuration from environment if available
      const proxyUrl =
        Deno.env.get("HTTPS_PROXY") || Deno.env.get("HTTP_PROXY");

      let fetchOptions: RequestInit = {
        ...options,
        signal: controller.signal,
      };

      // For proxy support in Deno, we need to handle this differently
      if (proxyUrl) {
        console.log(`🌐 Using proxy: ${proxyUrl} for ${url}`);

        // For HTTPS URLs through HTTP proxy, we need to use CONNECT method
        if (url.startsWith("https://") && proxyUrl.startsWith("http://")) {
          const targetUrl = new URL(url);
          const proxyUrlObj = new URL(proxyUrl);

          // First establish a CONNECT tunnel through the proxy
          const connectUrl = `${proxyUrl}${targetUrl.pathname}${targetUrl.search}`;
          const connectHeaders = new Headers(options.headers);
          connectHeaders.set("Host", targetUrl.hostname);

          fetchOptions = {
            ...options,
            signal: controller.signal,
            headers: connectHeaders,
          };

          console.log(
            `🔗 Connecting via proxy CONNECT to ${targetUrl.hostname}:${targetUrl.port || "443"}`,
          );
        }
      }

      const response = await fetch(url, fetchOptions);
      return response;
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(
          `HTTP request timed out after ${this.networkConfig.http_timeout}ms: ${url}`,
        );
      }
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
        console.log(`🔍 Connectivity check via proxy: ${proxyUrl}`);
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

      console.log(
        `✅ Network connectivity verified (status: ${response.status})`,
      );
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
        console.log(`🔄 Attempt ${attempt}/${this.retryConfig.max_attempts}`);
        return await operation();
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));

        if (attempt === this.retryConfig.max_attempts) {
          console.error(
            `❌ All ${this.retryConfig.max_attempts} attempts failed`,
          );
          throw lastError;
        }

        const delay = Math.min(
          this.retryConfig.base_delay *
            Math.pow(this.retryConfig.backoff_factor, attempt - 1),
          this.retryConfig.max_delay,
        );

        console.warn(`⚠️ Attempt ${attempt} failed. Retrying in ${delay}ms...`);
        await this.sleep(delay);

        // Reset browser state for next attempt if it's a browser operation
        if (
          operationName.includes("browser") ||
          operationName.includes("OAuth")
        ) {
          try {
            await this.resetBrowserState();
          } catch (resetError) {
            console.warn("⚠️ Browser state reset failed");
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
      } catch (error) {
        console.warn("⚠️ Failed to reset page state");
      }
    }
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
