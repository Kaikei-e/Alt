import { test as base, expect } from "@playwright/test";
import { DesktopPage, LoginPage } from "../pages";

/**
 * Test fixtures with page objects and enhanced test isolation
 */
export const test = base.extend<{
  loginPage: LoginPage;
  desktopPage: DesktopPage;
}>({
  // Enhanced page fixture with automatic cleanup and isolation
  page: async ({ browser }, use) => {
    // Create a new context with clean state for each test
    const context = await browser.newContext({
      // Disable caching for test isolation
      storageState: undefined,
    });

    // Clear any existing storage
    await context.clearCookies();
    await context.clearPermissions();

    const page = await context.newPage();

    // Add console error tracking for debugging
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") {
        consoleErrors.push(msg.text());
      }
    });

    // Add error tracking
    const pageErrors: Error[] = [];
    page.on("pageerror", (error) => {
      pageErrors.push(error);
    });

    // Expose error arrays for test debugging
    (page as any).__testErrors = { consoleErrors, pageErrors };

    try {
      await use(page);
    } finally {
      // Log errors if any occurred (for debugging)
      if (consoleErrors.length > 0 || pageErrors.length > 0) {
        console.log(`[TEST-CLEANUP] Errors during test:
          Console errors: ${consoleErrors.length}
          Page errors: ${pageErrors.length}`);
      }

      // Clean up
      await context.close();
    }
  },

  loginPage: async ({ page }, use) => {
    const loginPage = new LoginPage(page);
    await use(loginPage);
  },

  desktopPage: async ({ page }, use) => {
    const desktopPage = new DesktopPage(page);
    await use(desktopPage);
  },
});

/**
 * Enhanced expect with custom matchers for auth flows
 */
const customExpect = base.expect.extend({
  async toBeOnAuthFlow(page: any, flowPattern = /\/auth\/login\?flow=/) {
    const url = page.url();
    const pass = flowPattern.test(url);

    if (pass) {
      return {
        message: () => `Expected page not to be on auth flow, but was on: ${url}`,
        pass: true,
      };
    } else {
      return {
        message: () =>
          `Expected page to be on auth flow matching ${flowPattern}, but was on: ${url}`,
        pass: false,
      };
    }
  },

  async toHaveCompletedAuth(page: any, expectedDestination: string | RegExp) {
    const url = page.url();
    const pass =
      typeof expectedDestination === "string"
        ? url === expectedDestination
        : expectedDestination.test(url);

    if (pass) {
      return {
        message: () =>
          `Expected page not to have completed auth to ${expectedDestination}, but was on: ${url}`,
        pass: true,
      };
    } else {
      return {
        message: () =>
          `Expected page to have completed auth to ${expectedDestination}, but was on: ${url}`,
        pass: false,
      };
    }
  },
});

export { customExpect as expect };
