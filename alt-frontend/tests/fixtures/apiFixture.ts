import { test as base, expect } from "@playwright/test";
import { setupAPIErrorMocks, setupBackendAPIMocks, setupSlowAPIMocks } from "../helpers/apiMocks";
import { DesktopPage, LoginPage } from "../pages";

/**
 * Extended test fixtures with automatic API mocking
 * This provides consistent mocking across all tests
 */
export const test = base.extend<{
  // Page objects
  loginPage: LoginPage;
  desktopPage: DesktopPage;

  // API mocking helpers
  mockAPI: boolean;
  errorAPI: boolean;
  slowAPI: boolean;
}>({
  loginPage: async ({ page }, use) => {
    const loginPage = new LoginPage(page);
    await use(loginPage);
  },

  desktopPage: async ({ page }, use) => {
    const desktopPage = new DesktopPage(page);
    await use(desktopPage);
  },

  // Auto-setup normal API mocks (default: true)
  mockAPI: [true, { option: true }],

  // Auto-setup error API mocks (default: false)
  errorAPI: [false, { option: true }],

  // Auto-setup slow API mocks (default: false)
  slowAPI: [false, { option: true }],

  // Override page fixture to automatically set up API mocks
  page: async ({ page, mockAPI, errorAPI, slowAPI }, use) => {
    // Set up API mocks based on options
    if (mockAPI && !errorAPI && !slowAPI) {
      await setupBackendAPIMocks(page);
    } else if (errorAPI) {
      await setupAPIErrorMocks(page);
    } else if (slowAPI) {
      await setupSlowAPIMocks(page);
    }

    await use(page);
  },
});

/**
 * Test fixture specifically for API-dependent component tests
 * Always sets up backend API mocks automatically
 */
export const componentTest = test.extend({
  mockAPI: true,
  errorAPI: false,
  slowAPI: false,
});

/**
 * Test fixture for error boundary testing
 * Sets up error API responses automatically
 */
export const errorTest = test.extend({
  mockAPI: false,
  errorAPI: true,
  slowAPI: false,
});

/**
 * Test fixture for performance/timeout testing
 * Sets up slow API responses automatically
 */
export const slowTest = test.extend({
  mockAPI: false,
  errorAPI: false,
  slowAPI: true,
});

export { expect } from "@playwright/test";
