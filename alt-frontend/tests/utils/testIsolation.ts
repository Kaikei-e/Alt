import type { Page } from "@playwright/test";

/**
 * Test isolation utilities for better test stability
 */

export class TestIsolationHelper {
  constructor(private page: Page) {}

  /**
   * Reset mock server state between tests
   */
  async resetMockServerState(): Promise<void> {
    const mockPort = process.env.PW_MOCK_PORT || "4545";

    try {
      // Reset mock server state if it has a reset endpoint
      await this.page.request.post(`http://localhost:${mockPort}/test/reset`, {
        ignoreHTTPSErrors: true,
        timeout: 5000,
      });
    } catch (_error) {
      // It's okay if the mock server doesn't have a reset endpoint
      // or if it's not responding - just continue
    }
  }

  /**
   * Clear all browser state for complete test isolation
   */
  async clearBrowserState(): Promise<void> {
    // Clear cookies
    await this.page.context().clearCookies();

    // Clear local storage and session storage
    await this.page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });

    // Clear permissions
    await this.page.context().clearPermissions();
  }

  /**
   * Wait for any pending network requests to complete
   */
  async waitForNetworkIdle(timeout = 5000): Promise<void> {
    await this.page.waitForLoadState("networkidle", { timeout });
  }

  /**
   * Complete test isolation setup - run before each test
   */
  async setup(): Promise<void> {
    await this.clearBrowserState();
    await this.resetMockServerState();
  }

  /**
   * Complete test isolation cleanup - run after each test
   */
  async cleanup(): Promise<void> {
    await this.clearBrowserState();

    // Close any open dialogs
    try {
      await this.page.waitForEvent("dialog", { timeout: 100 });
    } catch {
      // No dialogs open, which is expected
    }

    // Wait for any pending operations
    await this.waitForNetworkIdle(2000);
  }
}

/**
 * Retry wrapper with exponential backoff for flaky operations
 */
export async function withRetry<T>(
  operation: () => Promise<T>,
  options: {
    maxAttempts?: number;
    initialDelay?: number;
    backoffFactor?: number;
    maxDelay?: number;
    shouldRetry?: (error: Error) => boolean;
  } = {},
): Promise<T> {
  const {
    maxAttempts = 3,
    initialDelay = 1000,
    backoffFactor = 2,
    maxDelay = 10000,
    shouldRetry = () => true,
  } = options;

  let lastError: Error;
  let delay = initialDelay;

  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await operation();
    } catch (error) {
      lastError = error as Error;

      if (attempt === maxAttempts || !shouldRetry(lastError)) {
        throw lastError;
      }

      console.log(
        `[RETRY] Attempt ${attempt} failed, retrying in ${delay}ms:`,
        error instanceof Error ? error.message : String(error),
      );
      await new Promise((resolve) => setTimeout(resolve, delay));
      delay = Math.min(delay * backoffFactor, maxDelay);
    }
  }

  throw lastError!;
}

/**
 * Utility to verify test conditions with better error messages
 */
export class TestConditionVerifier {
  constructor(private page: Page) {}

  /**
   * Verify auth flow state with detailed debugging
   */
  async verifyAuthFlowState(
    expectedState: "login" | "authenticated" | "expired",
  ): Promise<void> {
    const url = this.page.url();
    const cookies = await this.page.context().cookies();

    console.log(`[AUTH-VERIFY] Current URL: ${url}`);
    console.log(`[AUTH-VERIFY] Cookies count: ${cookies.length}`);

    const sessionCookie = cookies.find((c) => c.name.includes("session"));
    if (sessionCookie) {
      console.log(`[AUTH-VERIFY] Session cookie found: ${sessionCookie.name}`);
    }

    switch (expectedState) {
      case "login":
        if (!/\/auth\/login/.test(url)) {
          throw new Error(`Expected to be on login page, but URL is: ${url}`);
        }
        break;
      case "authenticated":
        if (!/\/desktop/.test(url)) {
          throw new Error(
            `Expected to be authenticated (on desktop), but URL is: ${url}`,
          );
        }
        if (!sessionCookie) {
          throw new Error(
            `Expected session cookie but none found. URL: ${url}`,
          );
        }
        break;
      case "expired":
        if (!/\/auth\/login\?flow=/.test(url)) {
          throw new Error(`Expected expired flow redirect, but URL is: ${url}`);
        }
        break;
    }
  }

  /**
   * Verify page is fully loaded and interactive
   */
  async verifyPageReady(): Promise<void> {
    await this.page.waitForLoadState("domcontentloaded");
    await this.page.waitForLoadState("networkidle");

    // Verify page is interactive
    const readyState = await this.page.evaluate(() => document.readyState);
    if (readyState !== "complete") {
      throw new Error(`Page not fully loaded. Ready state: ${readyState}`);
    }
  }
}
