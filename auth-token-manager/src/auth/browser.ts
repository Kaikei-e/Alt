/**
 * Browser Management System for OAuth Automation
 * 
 * This module provides robust browser automation capabilities using Playwright
 * for OAuth flows, with support for debugging, error recovery, and session management.
 */

import { chromium, firefox, webkit } from 'npm:playwright@^1.48.0';
import type { Browser, BrowserContext, Page } from 'npm:playwright@^1.48.0';
import type {
  BrowserConfig,
  BrowserSession,
  ConsoleLog,
  NetworkRequest,
  ProxyConfig,
  AuthTokenError,
  BrowserAutomationError
} from './types.ts';
import { logger } from '../utils/logger.ts';
import { retryWithBackoff } from '../utils/retry.ts';

/**
 * Browser automation manager with comprehensive session tracking
 */
export class BrowserManager {
  private browser: Browser | null = null;
  private context: BrowserContext | null = null;
  private page: Page | null = null;
  private session: BrowserSession | null = null;
  private config: BrowserConfig;

  constructor(config: BrowserConfig) {
    this.config = config;
  }

  /**
   * Initialize browser with configuration and prepare for automation
   */
  async initialize(): Promise<void> {
    try {
      await this.launchBrowser();
      await this.createContext();
      await this.createPage();
      this.initializeSession();
      
      logger.info('Browser manager initialized successfully', {
        browser_type: this.config.browser_type,
        headless: this.config.headless,
        session_id: this.session?.session_id
      });
    } catch (error) {
      const browserError: BrowserAutomationError = {
        type: 'browser_automation_error',
        code: 'BROWSER_LAUNCH_FAILED',
        message: `Failed to initialize browser: ${error.message}`,
        details: {
          timeout: this.config.launch_timeout
        }
      };
      
      logger.error('Browser initialization failed', browserError);
      throw browserError;
    }
  }

  /**
   * Launch browser instance with configuration
   */
  private async launchBrowser(): Promise<void> {
    const launchOptions = {
      headless: this.config.headless,
      timeout: this.config.launch_timeout,
      slowMo: this.config.debug.slow_mo,
      proxy: this.config.proxy ? {
        server: `${this.config.proxy.protocol}://${this.config.proxy.server}:${this.config.proxy.port}`,
        username: this.config.proxy.auth?.username,
        password: this.config.proxy.auth?.password,
        bypass: this.config.proxy.bypass?.join(',')
      } : undefined,
      args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',
        '--disable-background-timer-throttling',
        '--disable-backgrounding-occluded-windows',
        '--disable-renderer-backgrounding'
      ]
    };

    switch (this.config.browser_type) {
      case 'chromium':
        this.browser = await chromium.launch(launchOptions);
        break;
      case 'firefox':
        this.browser = await firefox.launch(launchOptions);
        break;
      case 'webkit':
        this.browser = await webkit.launch(launchOptions);
        break;
      default:
        throw new Error(`Unsupported browser type: ${this.config.browser_type}`);
    }
  }

  /**
   * Create browser context with security and automation settings
   */
  private async createContext(): Promise<void> {
    if (!this.browser) {
      throw new Error('Browser not initialized');
    }

    const contextOptions = {
      viewport: this.config.viewport,
      userAgent: this.config.user_agent,
      acceptDownloads: false,
      ignoreHTTPSErrors: true,
      recordVideo: this.config.debug.record_video ? {
        dir: './videos/',
        size: this.config.viewport
      } : undefined,
      recordHar: this.config.debug.record_video ? {
        path: './har/session.har'
      } : undefined
    };

    this.context = await this.browser.newContext(contextOptions);
    
    // Set up request/response logging
    this.context.on('request', (request) => {
      if (this.session) {
        const networkRequest: NetworkRequest = {
          url: request.url(),
          method: request.method(),
          status: 0, // Will be updated on response
          duration: 0, // Will be calculated on response
          timestamp: Date.now(),
          headers: request.headers(),
          response_headers: {}
        };
        this.session.network_requests.push(networkRequest);
      }
    });

    this.context.on('response', (response) => {
      if (this.session) {
        const requests = this.session.network_requests;
        const request = requests.find(req => req.url === response.url() && req.status === 0);
        if (request) {
          request.status = response.status();
          request.duration = Date.now() - request.timestamp;
          request.response_headers = response.headers();
        }
      }
    });
  }

  /**
   * Create new page and set up event listeners
   */
  private async createPage(): Promise<void> {
    if (!this.context) {
      throw new Error('Browser context not initialized');
    }

    this.page = await this.context.newPage();
    
    // Set navigation timeout
    this.page.setDefaultNavigationTimeout(this.config.navigation_timeout);
    this.page.setDefaultTimeout(this.config.navigation_timeout);

    // Set up console logging
    this.page.on('console', (msg) => {
      if (this.session) {
        const consoleLog: ConsoleLog = {
          level: msg.type(),
          message: msg.text(),
          timestamp: Date.now(),
          location: msg.location().url
        };
        this.session.console_logs.push(consoleLog);
      }
    });

    // Set up error logging
    this.page.on('pageerror', (error) => {
      logger.error('Page error occurred', {
        error: error.message,
        stack: error.stack,
        session_id: this.session?.session_id
      });
    });

    // Track page navigation
    this.page.on('framenavigated', (frame) => {
      if (frame === this.page?.mainFrame() && this.session) {
        this.session.current_url = frame.url();
        this.session.last_activity = Date.now();
      }
    });
  }

  /**
   * Initialize browser session tracking
   */
  private initializeSession(): void {
    this.session = {
      session_id: `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      current_url: 'about:blank',
      is_active: true,
      started_at: Date.now(),
      last_activity: Date.now(),
      screenshots: [],
      console_logs: [],
      network_requests: []
    };
  }

  /**
   * Navigate to URL with retry logic and error handling
   */
  async navigateTo(url: string): Promise<void> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    const operation = async () => {
      const response = await this.page!.goto(url, {
        waitUntil: 'networkidle',
        timeout: this.config.navigation_timeout
      });

      if (!response || !response.ok()) {
        throw new Error(`Navigation failed with status: ${response?.status()}`);
      }

      this.session!.current_url = url;
      this.session!.last_activity = Date.now();

      logger.info('Successfully navigated to URL', {
        url,
        session_id: this.session!.session_id,
        status: response.status()
      });
    };

    try {
      await retryWithBackoff(operation, {
        max_attempts: 3,
        initial_delay: 1000,
        max_delay: 5000,
        backoff_factor: 2,
        jitter: true,
        retryable_status_codes: [502, 503, 504],
        retryable_errors: ['timeout', 'network']
      });
    } catch (error) {
      const browserError: BrowserAutomationError = {
        type: 'browser_automation_error',
        code: 'PAGE_LOAD_TIMEOUT',
        message: `Failed to navigate to ${url}: ${error.message}`,
        details: {
          session_id: this.session.session_id,
          url,
          timeout: this.config.navigation_timeout
        }
      };

      await this.captureScreenshot('navigation_failed');
      logger.error('Navigation failed', browserError);
      throw browserError;
    }
  }

  /**
   * Wait for element with enhanced error reporting
   */
  async waitForElement(selector: string, timeout?: number): Promise<void> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    const waitTimeout = timeout || this.config.navigation_timeout;

    try {
      await this.page.waitForSelector(selector, {
        timeout: waitTimeout,
        state: 'visible'
      });

      this.session.last_activity = Date.now();

      logger.debug('Element found successfully', {
        selector,
        session_id: this.session.session_id,
        timeout: waitTimeout
      });
    } catch (error) {
      const browserError: BrowserAutomationError = {
        type: 'browser_automation_error',
        code: 'ELEMENT_NOT_FOUND',
        message: `Element not found: ${selector}`,
        details: {
          session_id: this.session.session_id,
          selector,
          timeout: waitTimeout,
          url: this.session.current_url
        }
      };

      await this.captureScreenshot('element_not_found');
      logger.error('Element not found', browserError);
      throw browserError;
    }
  }

  /**
   * Click element with retry logic
   */
  async clickElement(selector: string): Promise<void> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    try {
      await this.waitForElement(selector);
      await this.page.click(selector);
      
      this.session.last_activity = Date.now();

      logger.debug('Element clicked successfully', {
        selector,
        session_id: this.session.session_id
      });
    } catch (error) {
      await this.captureScreenshot('click_failed');
      throw error;
    }
  }

  /**
   * Fill input field with text
   */
  async fillInput(selector: string, text: string, options?: { delay?: number }): Promise<void> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    try {
      await this.waitForElement(selector);
      await this.page.fill(selector, text, {
        timeout: this.config.navigation_timeout
      });

      // Optional typing delay for more human-like interaction
      if (options?.delay) {
        await this.page.type(selector, text, { delay: options.delay });
      }

      this.session.last_activity = Date.now();

      logger.debug('Input filled successfully', {
        selector,
        text_length: text.length,
        session_id: this.session.session_id
      });
    } catch (error) {
      await this.captureScreenshot('fill_failed');
      throw error;
    }
  }

  /**
   * Get text content from element
   */
  async getElementText(selector: string): Promise<string> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    try {
      await this.waitForElement(selector);
      const text = await this.page.textContent(selector);
      
      this.session.last_activity = Date.now();

      logger.debug('Element text retrieved', {
        selector,
        text_length: text?.length || 0,
        session_id: this.session.session_id
      });

      return text || '';
    } catch (error) {
      await this.captureScreenshot('text_retrieval_failed');
      throw error;
    }
  }

  /**
   * Wait for URL to match pattern or change
   */
  async waitForUrl(pattern: string | RegExp, timeout?: number): Promise<string> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    const waitTimeout = timeout || this.config.oauth_timeout;

    try {
      await this.page.waitForURL(pattern, {
        timeout: waitTimeout,
        waitUntil: 'networkidle'
      });

      const currentUrl = this.page.url();
      this.session.current_url = currentUrl;
      this.session.last_activity = Date.now();

      logger.info('URL pattern matched', {
        pattern: pattern.toString(),
        current_url: currentUrl,
        session_id: this.session.session_id
      });

      return currentUrl;
    } catch (error) {
      const browserError: BrowserAutomationError = {
        type: 'browser_automation_error',
        code: 'AUTOMATION_TIMEOUT',
        message: `URL pattern not matched within timeout: ${pattern}`,
        details: {
          session_id: this.session.session_id,
          url: this.session.current_url,
          timeout: waitTimeout
        }
      };

      await this.captureScreenshot('url_timeout');
      logger.error('URL wait timeout', browserError);
      throw browserError;
    }
  }

  /**
   * Capture screenshot for debugging purposes
   */
  async captureScreenshot(label: string): Promise<string> {
    if (!this.page || !this.session) {
      throw new Error('Browser not initialized');
    }

    if (!this.config.debug.capture_screenshots) {
      return '';
    }

    try {
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
      const filename = `screenshot_${label}_${timestamp}.png`;
      const path = `./screenshots/${filename}`;

      await this.page.screenshot({
        path,
        fullPage: true
      });

      this.session.screenshots.push(path);

      logger.debug('Screenshot captured', {
        label,
        filename,
        session_id: this.session.session_id
      });

      return path;
    } catch (error) {
      logger.warn('Failed to capture screenshot', {
        label,
        error: error.message,
        session_id: this.session.session_id
      });
      return '';
    }
  }

  /**
   * Get current page URL
   */
  getCurrentUrl(): string {
    return this.session?.current_url || '';
  }

  /**
   * Get current browser session information
   */
  getSession(): BrowserSession | null {
    return this.session;
  }

  /**
   * Check if browser is currently active
   */
  isActive(): boolean {
    return this.session?.is_active || false;
  }

  /**
   * Extract URL parameters from current page
   */
  async extractUrlParams(): Promise<Record<string, string>> {
    if (!this.page) {
      throw new Error('Browser not initialized');
    }

    const url = this.page.url();
    const urlObj = new URL(url);
    const params: Record<string, string> = {};

    for (const [key, value] of urlObj.searchParams.entries()) {
      params[key] = value;
    }

    // Also check for hash parameters (for OAuth implicit flow)
    if (urlObj.hash) {
      const hashParams = new URLSearchParams(urlObj.hash.substring(1));
      for (const [key, value] of hashParams.entries()) {
        params[key] = value;
      }
    }

    logger.debug('URL parameters extracted', {
      url,
      param_count: Object.keys(params).length,
      session_id: this.session?.session_id
    });

    return params;
  }

  /**
   * Execute JavaScript in the browser context
   */
  async executeScript<T>(script: string): Promise<T> {
    if (!this.page) {
      throw new Error('Browser not initialized');
    }

    try {
      const result = await this.page.evaluate(script);
      
      if (this.session) {
        this.session.last_activity = Date.now();
      }

      logger.debug('Script executed successfully', {
        script_length: script.length,
        session_id: this.session?.session_id
      });

      return result as T;
    } catch (error) {
      await this.captureScreenshot('script_execution_failed');
      throw error;
    }
  }

  /**
   * Clean up browser resources
   */
  async cleanup(): Promise<void> {
    try {
      if (this.session) {
        this.session.is_active = false;
      }

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

      logger.info('Browser cleanup completed', {
        session_id: this.session?.session_id
      });
    } catch (error) {
      logger.error('Browser cleanup failed', {
        error: error.message,
        session_id: this.session?.session_id
      });
    }
  }

  /**
   * Get browser session metrics for monitoring
   */
  getSessionMetrics() {
    if (!this.session) {
      return null;
    }

    const now = Date.now();
    return {
      session_id: this.session.session_id,
      duration: now - this.session.started_at,
      last_activity_ago: now - this.session.last_activity,
      screenshot_count: this.session.screenshots.length,
      console_log_count: this.session.console_logs.length,
      network_request_count: this.session.network_requests.length,
      current_url: this.session.current_url,
      is_active: this.session.is_active
    };
  }
}