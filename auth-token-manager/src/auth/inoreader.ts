/**
 * Inoreader OAuth Automation with Envoy Proxy Support
 * 
 * This module handles the complete Inoreader OAuth flow using browser automation,
 * including login, consent, and token extraction. Supports Envoy proxy integration
 * for external API calls and comprehensive error recovery.
 */

import type {
  InoreaderOAuthConfig,
  OAuth2Token,
  OAuthResult,
  TokenSource,
  TokenValidationResult,
  BrowserSession,
  AuthTokenError,
  OAuthFlowError,
  NetworkError
} from './types.ts';
import { BrowserManager } from './browser.ts';
import { logger } from '../utils/logger.ts';
import { retryWithBackoff } from '../utils/retry.ts';

/**
 * Inoreader OAuth automation service with comprehensive flow management
 */
export class InoreaderOAuthService {
  private config: InoreaderOAuthConfig;
  private browserManager: BrowserManager;

  constructor(config: InoreaderOAuthConfig, browserManager: BrowserManager) {
    this.config = config;
    this.browserManager = browserManager;
  }

  /**
   * Execute complete OAuth flow using browser automation
   */
  async executeOAuthFlow(): Promise<OAuthResult> {
    const startTime = Date.now();
    let attempts = 0;
    let session: BrowserSession | null = null;

    try {
      logger.info('Starting Inoreader OAuth flow', {
        client_id: this.config.client_id,
        redirect_uri: this.config.redirect_uri,
        scope: this.config.scope
      });

      // Initialize browser session
      await this.browserManager.initialize();
      session = this.browserManager.getSession();

      // Navigate to OAuth authorization URL
      const authUrl = this.buildAuthorizationUrl();
      await this.browserManager.navigateTo(authUrl);
      await this.browserManager.captureScreenshot('oauth_start');

      // Handle login form
      await this.handleLogin();
      await this.browserManager.captureScreenshot('after_login');

      // Handle consent screen (if present)
      await this.handleConsent();
      await this.browserManager.captureScreenshot('after_consent');

      // Wait for redirect and extract authorization code
      const authCode = await this.extractAuthorizationCode();
      await this.browserManager.captureScreenshot('auth_code_received');

      // Exchange authorization code for access token
      const token = await this.exchangeCodeForToken(authCode);

      // Validate the received token
      const validationResult = await this.validateToken(token);
      if (!validationResult.is_valid) {
        throw new Error(`Token validation failed: ${validationResult.error}`);
      }

      const result: OAuthResult = {
        success: true,
        token,
        metadata: {
          duration: Date.now() - startTime,
          attempts: attempts + 1,
          method: 'browser_automation',
          session_id: session?.session_id,
          screenshots: session?.screenshots || []
        }
      };

      logger.info('OAuth flow completed successfully', {
        duration: result.metadata.duration,
        token_expires_at: token.expires_at,
        session_id: session?.session_id
      });

      return result;

    } catch (error) {
      const result: OAuthResult = {
        success: false,
        error: error.message,
        metadata: {
          duration: Date.now() - startTime,
          attempts: attempts + 1,
          method: 'browser_automation',
          session_id: session?.session_id,
          screenshots: session?.screenshots || []
        }
      };

      logger.error('OAuth flow failed', {
        error: error.message,
        duration: result.metadata.duration,
        session_id: session?.session_id
      });

      return result;

    } finally {
      // Always cleanup browser resources
      await this.browserManager.cleanup();
    }
  }

  /**
   * Build OAuth authorization URL with all required parameters
   */
  private buildAuthorizationUrl(): string {
    const params = new URLSearchParams({
      response_type: 'code',
      client_id: this.config.client_id,
      redirect_uri: this.config.redirect_uri,
      scope: this.config.scope,
      state: this.config.oauth_state
    });

    const authUrl = `${this.config.auth_url}?${params.toString()}`;
    
    logger.debug('Built authorization URL', {
      auth_url: this.config.auth_url,
      client_id: this.config.client_id,
      redirect_uri: this.config.redirect_uri,
      scope: this.config.scope,
      state: this.config.oauth_state
    });

    return authUrl;
  }

  /**
   * Handle Inoreader login form with credentials
   */
  private async handleLogin(): Promise<void> {
    try {
      // Wait for login form to appear
      await this.browserManager.waitForElement('input[name="Email"], input[type="email"], #Email', 10000);

      // Fill in username/email
      const emailSelectors = ['input[name="Email"]', 'input[type="email"]', '#Email', 'input[name="username"]'];
      let emailFilled = false;
      
      for (const selector of emailSelectors) {
        try {
          await this.browserManager.waitForElement(selector, 2000);
          await this.browserManager.fillInput(selector, this.config.credentials.username);
          emailFilled = true;
          logger.debug('Email filled successfully', { selector });
          break;
        } catch (error) {
          // Continue trying other selectors
          continue;
        }
      }

      if (!emailFilled) {
        throw new Error('Could not find email input field');
      }

      // Fill in password
      const passwordSelectors = ['input[name="Passwd"]', 'input[type="password"]', '#Passwd', 'input[name="password"]'];
      let passwordFilled = false;

      for (const selector of passwordSelectors) {
        try {
          await this.browserManager.waitForElement(selector, 2000);
          await this.browserManager.fillInput(selector, this.config.credentials.password);
          passwordFilled = true;
          logger.debug('Password filled successfully', { selector });
          break;
        } catch (error) {
          // Continue trying other selectors
          continue;
        }
      }

      if (!passwordFilled) {
        throw new Error('Could not find password input field');
      }

      // Submit login form
      const submitSelectors = [
        'input[type="submit"]',
        'button[type="submit"]',
        'input[value="Sign in"]',
        'button:has-text("Sign in")',
        'button:has-text("Login")',
        '#signIn'
      ];

      let formSubmitted = false;
      for (const selector of submitSelectors) {
        try {
          await this.browserManager.waitForElement(selector, 2000);
          await this.browserManager.clickElement(selector);
          formSubmitted = true;
          logger.debug('Login form submitted', { selector });
          break;
        } catch (error) {
          // Continue trying other selectors
          continue;
        }
      }

      if (!formSubmitted) {
        // Try pressing Enter on password field as fallback
        try {
          await this.browserManager.executeScript('document.querySelector("input[type=\'password\']").form.submit()');
          formSubmitted = true;
          logger.debug('Login form submitted via script');
        } catch (error) {
          throw new Error('Could not submit login form');
        }
      }

      // Wait for redirect or next step
      await new Promise(resolve => setTimeout(resolve, 3000));

      // Check for login errors
      const errorMessages = [
        'Wrong username or password',
        'Invalid credentials',
        'Login failed',
        'Authentication failed'
      ];

      for (const errorMsg of errorMessages) {
        try {
          const pageContent = await this.browserManager.executeScript<string>(
            'document.body.innerText'
          );
          if (pageContent.includes(errorMsg)) {
            const oauthError: OAuthFlowError = {
              type: 'oauth_flow_error',
              code: 'INVALID_CREDENTIALS',
              message: 'Login failed with provided credentials',
              details: {
                error_description: errorMsg
              }
            };
            throw oauthError;
          }
        } catch (error) {
          // If we can't check for errors, continue
          if (error.type === 'oauth_flow_error') {
            throw error;
          }
        }
      }

      logger.info('Login completed successfully');

    } catch (error) {
      await this.browserManager.captureScreenshot('login_failed');
      
      if (error.type === 'oauth_flow_error') {
        throw error;
      }

      const oauthError: OAuthFlowError = {
        type: 'oauth_flow_error',
        code: 'INVALID_CREDENTIALS',
        message: `Login process failed: ${error.message}`,
        details: {}
      };
      
      logger.error('Login failed', oauthError);
      throw oauthError;
    }
  }

  /**
   * Handle OAuth consent screen if present
   */
  private async handleConsent(): Promise<void> {
    try {
      // Look for common consent screen elements
      const consentSelectors = [
        'button:has-text("Allow")',
        'button:has-text("Authorize")',
        'button:has-text("Grant")',
        'input[value="Allow"]',
        'button[name="allow"]',
        '#allow_button'
      ];

      let consentHandled = false;
      
      // Wait briefly to see if consent screen appears
      await new Promise(resolve => setTimeout(resolve, 2000));

      for (const selector of consentSelectors) {
        try {
          await this.browserManager.waitForElement(selector, 3000);
          await this.browserManager.clickElement(selector);
          consentHandled = true;
          logger.info('Consent granted successfully', { selector });
          break;
        } catch (error) {
          // Continue trying other selectors
          continue;
        }
      }

      if (!consentHandled) {
        // Check if we're already past consent (no consent screen shown)
        const currentUrl = this.browserManager.getCurrentUrl();
        if (currentUrl.includes(this.config.redirect_uri) || currentUrl.includes('code=')) {
          logger.info('No consent screen found, proceeding to authorization code extraction');
          return;
        }
        
        logger.warn('Consent screen expected but not found, proceeding anyway');
      }

      // Wait for redirect after consent
      await new Promise(resolve => setTimeout(resolve, 2000));

    } catch (error) {
      await this.browserManager.captureScreenshot('consent_failed');
      
      // Don't fail the entire flow if consent handling fails
      // Some OAuth flows might not have a consent screen
      logger.warn('Consent handling failed, continuing with flow', {
        error: error.message
      });
    }
  }

  /**
   * Extract authorization code from redirect URL
   */
  private async extractAuthorizationCode(): Promise<string> {
    try {
      // Wait for redirect to our callback URL
      const redirectPattern = new RegExp(`${this.config.redirect_uri.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`);
      
      await this.browserManager.waitForUrl(redirectPattern, 30000);

      // Extract URL parameters
      const urlParams = await this.browserManager.extractUrlParams();

      // Validate state parameter for CSRF protection
      if (urlParams.state !== this.config.oauth_state) {
        const oauthError: OAuthFlowError = {
          type: 'oauth_flow_error',
          code: 'STATE_MISMATCH',
          message: 'OAuth state parameter mismatch',
          details: {
            oauth_state: this.config.oauth_state,
            redirect_uri: this.config.redirect_uri
          }
        };
        throw oauthError;
      }

      // Check for OAuth error response
      if (urlParams.error) {
        const oauthError: OAuthFlowError = {
          type: 'oauth_flow_error',
          code: 'OAUTH_DENIED',
          message: `OAuth authorization failed: ${urlParams.error}`,
          details: {
            error_description: urlParams.error_description || 'No description provided'
          }
        };
        throw oauthError;
      }

      // Extract authorization code
      const authCode = urlParams.code;
      if (!authCode) {
        const oauthError: OAuthFlowError = {
          type: 'oauth_flow_error',
          code: 'TOKEN_EXCHANGE_FAILED',
          message: 'Authorization code not found in redirect URL',
          details: {
            redirect_uri: this.browserManager.getCurrentUrl()
          }
        };
        throw oauthError;
      }

      logger.info('Authorization code extracted successfully', {
        code_length: authCode.length,
        redirect_url: this.browserManager.getCurrentUrl()
      });

      return authCode;

    } catch (error) {
      await this.browserManager.captureScreenshot('auth_code_extraction_failed');
      
      if (error.type === 'oauth_flow_error') {
        throw error;
      }

      const oauthError: OAuthFlowError = {
        type: 'oauth_flow_error',
        code: 'TOKEN_EXCHANGE_FAILED',
        message: `Failed to extract authorization code: ${error.message}`,
        details: {}
      };
      
      logger.error('Authorization code extraction failed', oauthError);
      throw oauthError;
    }
  }

  /**
   * Exchange authorization code for access token via Envoy proxy
   */
  private async exchangeCodeForToken(authCode: string): Promise<OAuth2Token> {
    const tokenRequestBody = new URLSearchParams({
      grant_type: 'authorization_code',
      client_id: this.config.client_id,
      client_secret: this.config.client_secret,
      code: authCode,
      redirect_uri: this.config.redirect_uri
    });

    const requestOptions = {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
        'User-Agent': 'Alt-RSS-Reader-Auth-Token-Manager/1.0'
      },
      body: tokenRequestBody.toString()
    };

    const operation = async () => {
      const response = await fetch(this.config.token_url, requestOptions);

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Token exchange failed: ${response.status} - ${errorText}`);
      }

      const tokenData = await response.json();

      // Validate required token fields
      if (!tokenData.access_token) {
        throw new Error('Access token not found in response');
      }

      const now = Date.now();
      const expiresIn = tokenData.expires_in || 3600; // Default to 1 hour if not specified

      const token: OAuth2Token = {
        access_token: tokenData.access_token,
        token_type: 'Bearer',
        expires_at: now + (expiresIn * 1000),
        refresh_token: tokenData.refresh_token,
        scope: tokenData.scope || this.config.scope,
        created_at: now,
        last_refreshed_at: now,
        refresh_count: 0,
        is_active: true,
        source: 'browser_automation'
      };

      logger.info('Token exchange completed successfully', {
        expires_in: expiresIn,
        expires_at: token.expires_at,
        has_refresh_token: !!token.refresh_token,
        scope: token.scope
      });

      return token;
    };

    try {
      return await retryWithBackoff(operation, {
        max_attempts: 3,
        initial_delay: 1000,
        max_delay: 5000,
        backoff_factor: 2,
        jitter: true,
        retryable_status_codes: [429, 500, 502, 503, 504],
        retryable_errors: ['timeout', 'network', 'fetch']
      });

    } catch (error) {
      const networkError: NetworkError = {
        type: 'network_error',
        code: 'CONNECTION_TIMEOUT',
        message: `Token exchange via Envoy proxy failed: ${error.message}`,
        details: {
          url: this.config.token_url
        }
      };

      logger.error('Token exchange failed', networkError);
      throw networkError;
    }
  }

  /**
   * Refresh existing OAuth token using refresh token
   */
  async refreshToken(currentToken: OAuth2Token): Promise<OAuthResult> {
    const startTime = Date.now();

    if (!currentToken.refresh_token) {
      return {
        success: false,
        error: 'No refresh token available',
        metadata: {
          duration: Date.now() - startTime,
          attempts: 1,
          method: 'api_refresh'
        }
      };
    }

    try {
      logger.info('Starting token refresh', {
        current_expires_at: currentToken.expires_at,
        refresh_count: currentToken.refresh_count
      });

      const refreshRequestBody = new URLSearchParams({
        grant_type: 'refresh_token',
        client_id: this.config.client_id,
        client_secret: this.config.client_secret,
        refresh_token: currentToken.refresh_token
      });

      const requestOptions = {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
          'User-Agent': 'Alt-RSS-Reader-Auth-Token-Manager/1.0'
        },
        body: refreshRequestBody.toString()
      };

      const operation = async () => {
        const response = await fetch(this.config.token_url, requestOptions);

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(`Token refresh failed: ${response.status} - ${errorText}`);
        }

        const tokenData = await response.json();

        if (!tokenData.access_token) {
          throw new Error('Access token not found in refresh response');
        }

        const now = Date.now();
        const expiresIn = tokenData.expires_in || 3600;

        const refreshedToken: OAuth2Token = {
          ...currentToken,
          access_token: tokenData.access_token,
          expires_at: now + (expiresIn * 1000),
          refresh_token: tokenData.refresh_token || currentToken.refresh_token,
          scope: tokenData.scope || currentToken.scope,
          last_refreshed_at: now,
          refresh_count: currentToken.refresh_count + 1,
          source: 'api_refresh'
        };

        return refreshedToken;
      };

      const refreshedToken = await retryWithBackoff(operation, {
        max_attempts: 3,
        initial_delay: 1000,
        max_delay: 5000,
        backoff_factor: 2,
        jitter: true,
        retryable_status_codes: [429, 500, 502, 503, 504],
        retryable_errors: ['timeout', 'network', 'fetch']
      });

      const result: OAuthResult = {
        success: true,
        token: refreshedToken,
        metadata: {
          duration: Date.now() - startTime,
          attempts: 1,
          method: 'api_refresh'
        }
      };

      logger.info('Token refresh completed successfully', {
        old_expires_at: currentToken.expires_at,
        new_expires_at: refreshedToken.expires_at,
        refresh_count: refreshedToken.refresh_count,
        duration: result.metadata.duration
      });

      return result;

    } catch (error) {
      const result: OAuthResult = {
        success: false,
        error: error.message,
        metadata: {
          duration: Date.now() - startTime,
          attempts: 1,
          method: 'api_refresh'
        }
      };

      logger.error('Token refresh failed', {
        error: error.message,
        current_token_expires_at: currentToken.expires_at,
        refresh_count: currentToken.refresh_count
      });

      return result;
    }
  }

  /**
   * Validate token by making a test API call to Inoreader
   */
  async validateToken(token: OAuth2Token): Promise<TokenValidationResult> {
    const now = Date.now();
    const isExpired = token.expires_at <= now;
    const expiresIn = Math.max(0, Math.floor((token.expires_at - now) / 1000));
    const needsRefresh = expiresIn < 300; // Refresh if less than 5 minutes remaining

    if (isExpired) {
      return {
        is_valid: false,
        is_expired: true,
        needs_refresh: true,
        expires_in: 0,
        error: 'Token is expired',
        metadata: {
          validated_at: now,
          token_age: now - token.created_at
        }
      };
    }

    try {
      // Test token by making API call to Inoreader user info endpoint
      const response = await fetch('https://www.inoreader.com/reader/api/0/user-info', {
        headers: {
          'Authorization': `Bearer ${token.access_token}`,
          'User-Agent': 'Alt-RSS-Reader-Auth-Token-Manager/1.0'
        }
      });

      const isValid = response.ok;

      return {
        is_valid: isValid,
        is_expired: false,
        needs_refresh: needsRefresh,
        expires_in: expiresIn,
        error: isValid ? undefined : `API validation failed: ${response.status}`,
        metadata: {
          validated_at: now,
          token_age: now - token.created_at,
          refresh_in: needsRefresh ? 0 : Math.max(0, expiresIn - 300)
        }
      };

    } catch (error) {
      return {
        is_valid: false,
        is_expired: false,
        needs_refresh: true,
        expires_in: expiresIn,
        error: `Token validation failed: ${error.message}`,
        metadata: {
          validated_at: now,
          token_age: now - token.created_at
        }
      };
    }
  }

  /**
   * Get current OAuth configuration (without sensitive data)
   */
  getConfig() {
    return {
      client_id: this.config.client_id,
      redirect_uri: this.config.redirect_uri,
      scope: this.config.scope,
      auth_url: this.config.auth_url,
      token_url: this.config.token_url,
      oauth_state: this.config.oauth_state,
      has_credentials: !!(this.config.credentials.username && this.config.credentials.password)
    };
  }
}