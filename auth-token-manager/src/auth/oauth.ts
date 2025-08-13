/**
 * Inoreader OAuth automation using Playwright
 */

import { Browser, BrowserContext, Page } from 'playwright';
import type { 
  InoreaderCredentials, 
  BrowserConfig, 
  TokenResponse, 
  AuthenticationResult,
  AuthError 
} from './types.ts';

export class InoreaderOAuthAutomator {
  private browser: Browser | null = null;
  private context: BrowserContext | null = null;
  private page: Page | null = null;

  constructor(
    private credentials: InoreaderCredentials,
    private browserConfig: BrowserConfig
  ) {}

  async initializeBrowser(): Promise<void> {
    try {
      // Import playwright dynamically for Deno compatibility
      const { chromium } = await import('playwright');
      
      console.log('🔧 Initializing browser...');
      
      this.browser = await chromium.launch({
        headless: this.browserConfig.headless,
        args: this.browserConfig.args
      });

      this.context = await this.browser.newContext({
        viewport: this.browserConfig.viewport,
        userAgent: this.browserConfig.user_agent,
        locale: this.browserConfig.locale,
        timezoneId: this.browserConfig.timezone,
        ignoreHTTPSErrors: true
      });

      this.page = await this.context.newPage();
      
      console.log('✅ Browser initialized successfully');
    } catch (error) {
      throw new Error(`Failed to initialize browser: ${error}`);
    }
  }

  async performOAuth(): Promise<AuthenticationResult> {
    const startTime = Date.now();
    
    try {
      // First try refresh token approach if available
      console.log('🔄 Attempting refresh token flow first...');
      
      try {
        const refreshResult = await this.tryRefreshTokenFlow();
        if (refreshResult.success) {
          const duration = Date.now() - startTime;
          console.log(`✅ Refresh token flow completed successfully in ${duration}ms`);
          return refreshResult;
        }
        console.log('⚠️ Refresh token flow failed, falling back to browser automation...');
      } catch (refreshError) {
        console.log('⚠️ Refresh token not available, using browser automation...');
      }

      // Fallback to browser automation flow
      if (!this.page) {
        throw new Error('Browser not initialized. Call initializeBrowser() first.');
      }

      console.log('🔐 Starting OAuth browser flow...');

      // Step 1: Navigate to Inoreader OAuth authorization
      const authUrl = this.buildAuthUrl();
      console.log('📍 Navigating to authorization URL');
      await this.page.goto(authUrl, { waitUntil: 'networkidle' });

      // Step 2: Handle login form
      console.log('🔑 Handling login form...');
      await this.handleLoginForm();

      // Step 3: Handle authorization consent
      console.log('✅ Handling authorization consent...');
      await this.handleAuthorizationConsent();

      // Step 4: Capture authorization code from redirect
      console.log('📋 Capturing authorization code...');
      const authCode = await this.captureAuthorizationCode();

      // Step 5: Exchange authorization code for tokens
      console.log('🔄 Exchanging code for tokens...');
      const tokens = await this.exchangeCodeForTokens(authCode);

      const duration = Date.now() - startTime;
      console.log(`✅ OAuth flow completed successfully in ${duration}ms`);

      return {
        success: true,
        tokens,
        metadata: {
          duration,
          user_agent: this.browserConfig.user_agent || 'unknown',
          session_id: crypto.randomUUID()
        }
      };

    } catch (error) {
      const duration = Date.now() - startTime;
      const errorMessage = error instanceof Error ? error.message : String(error);
      
      console.error(`❌ OAuth flow failed after ${duration}ms:`, errorMessage);
      
      return {
        success: false,
        error: errorMessage,
        metadata: {
          duration,
          user_agent: this.browserConfig.user_agent || 'unknown',
          session_id: crypto.randomUUID()
        }
      };
    }
  }

  private buildAuthUrl(): string {
    const params = new URLSearchParams({
      response_type: 'code',
      client_id: this.credentials.client_id,
      redirect_uri: this.credentials.redirect_uri,
      scope: 'read write',
      state: crypto.randomUUID()
    });

    return `https://www.inoreader.com/oauth2/auth?${params.toString()}`;
  }

  private async handleLoginForm(): Promise<void> {
    if (!this.page) throw new Error('Page not initialized');

    // Wait for login form
    await this.page.waitForSelector('#email', { timeout: 10000 });

    // Fill credentials
    await this.page.fill('#email', this.credentials.username);
    await this.page.fill('#passwd', this.credentials.password);

    // Submit form
    await this.page.click('input[type="submit"]');
    
    // Wait for navigation
    await this.page.waitForNavigation({ waitUntil: 'networkidle' });
  }

  private async handleAuthorizationConsent(): Promise<void> {
    if (!this.page) throw new Error('Page not initialized');

    try {
      // Look for authorization consent button
      const consentButton = await this.page.waitForSelector(
        'input[value="Allow"], button:has-text("Allow"), input[value="Authorize"], button:has-text("Authorize")',
        { timeout: 5000 }
      );

      if (consentButton) {
        console.log('🎯 Found consent button, clicking...');
        await consentButton.click();
        await this.page.waitForNavigation({ waitUntil: 'networkidle' });
      }
    } catch (error) {
      console.log('ℹ️ No consent page found, proceeding...');
    }
  }

  private async captureAuthorizationCode(): Promise<string> {
    if (!this.page) throw new Error('Page not initialized');

    // Check if using OOB (out-of-band) flow
    if (this.credentials.redirect_uri === 'urn:ietf:wg:oauth:2.0:oob') {
      // For OOB flow, look for the authorization code in the page content
      try {
        // Wait for the authorization code to appear on the page
        await this.page.waitForSelector('input[readonly], code, .code', { timeout: 15000 });
        
        // Try different selectors to find the authorization code
        const codeElement = await this.page.$('input[readonly]') || 
                           await this.page.$('code') || 
                           await this.page.$('.code') ||
                           await this.page.$('[data-code]');
        
        if (codeElement) {
          const code = await codeElement.textContent() || await codeElement.getAttribute('value');
          if (code && code.trim()) {
            console.log('📋 Found authorization code via OOB flow');
            return code.trim();
          }
        }
        
        // Fallback: look for code in page text
        const pageContent = await this.page.textContent('body') || '';
        const codeMatch = pageContent.match(/\b[A-Za-z0-9]{20,}\b/);
        if (codeMatch) {
          console.log('📋 Found authorization code in page content');
          return codeMatch[0];
        }
        
        throw new Error('Authorization code not found in OOB response');
      } catch (error) {
        console.error('🔍 OOB code capture failed, trying URL-based capture');
        // Fallback to URL-based capture
      }
    }
    
    // Standard callback URL flow
    await this.page.waitForURL(/callback|code=/, { timeout: 15000 });
    
    const url = this.page.url();
    const urlObj = new URL(url);
    const code = urlObj.searchParams.get('code');

    if (!code) {
      throw new Error('Authorization code not found in callback URL');
    }

    return code;
  }

  private async exchangeCodeForTokens(code: string): Promise<TokenResponse> {
    const tokenUrl = 'https://www.inoreader.com/oauth2/token';
    
    const body = new URLSearchParams({
      grant_type: 'authorization_code',
      client_id: this.credentials.client_id,
      client_secret: this.credentials.client_secret,
      code: code,
      redirect_uri: this.credentials.redirect_uri
    });

    const response = await fetch(tokenUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'User-Agent': this.browserConfig.user_agent || 'Auth-Token-Manager/2.0.0'
      },
      body: body.toString()
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Token exchange failed: ${response.status} ${errorText}`);
    }

    const data = await response.json();
    
    const expiresAt = new Date(Date.now() + (data.expires_in * 1000));

    return {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expires_at: expiresAt,
      token_type: data.token_type,
      scope: data.scope
    };
  }

  private async tryRefreshTokenFlow(): Promise<AuthenticationResult> {
    try {
      // Import K8s secret manager to get existing refresh token
      const { K8sSecretManager } = await import('../k8s/secret-manager-simple.ts');
      const secretManager = new K8sSecretManager('alt-processing', 'pre-processor-sidecar-oauth2-token');
      
      // Try to get existing token data
      const existingTokenData = await secretManager.getTokenSecret();
      if (!existingTokenData || !existingTokenData.refresh_token) {
        throw new Error('No existing refresh token found');
      }

      console.log('🔄 Found existing refresh token, attempting refresh...');
      
      // Use refresh token to get new access token
      const response = await fetch('https://www.inoreader.com/oauth2/token', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'User-Agent': this.browserConfig.user_agent || 'Auth-Token-Manager/2.0.0'
        },
        body: new URLSearchParams({
          grant_type: 'refresh_token',
          client_id: this.credentials.client_id,
          client_secret: this.credentials.client_secret,
          refresh_token: existingTokenData.refresh_token
        })
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Refresh token failed: ${response.status} ${errorText}`);
      }

      const data = await response.json();
      const expiresAt = new Date(Date.now() + (data.expires_in * 1000));

      const tokens: TokenResponse = {
        access_token: data.access_token,
        refresh_token: data.refresh_token,
        expires_at: expiresAt,
        token_type: data.token_type || 'Bearer',
        scope: data.scope || 'read write'
      };

      console.log('✅ Refresh token flow successful');
      
      return {
        success: true,
        tokens,
        metadata: {
          duration: 0,
          user_agent: this.browserConfig.user_agent || 'unknown',
          session_id: crypto.randomUUID(),
          method: 'refresh_token'
        }
      };
      
    } catch (error) {
      console.log(`⚠️ Refresh token flow failed: ${error}`);
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
      
      console.log('🧹 Browser cleanup completed');
    } catch (error) {
      console.warn('⚠️ Error during browser cleanup:', error);
    }
  }
}