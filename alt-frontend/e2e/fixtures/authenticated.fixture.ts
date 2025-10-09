import { test as base } from '@playwright/test';
import type { Page } from '@playwright/test';

/**
 * Authenticated user fixture
 * Extends base test with authenticated page context
 */
export type AuthenticatedFixture = {
  authenticatedPage: Page;
};

/**
 * Test fixture for authenticated scenarios
 * Uses the storageState from setup to provide an authenticated page
 */
export const test = base.extend<AuthenticatedFixture>({
  authenticatedPage: async ({ page }, use) => {
    // The page is already authenticated via storageState in playwright.config.ts
    // This fixture just provides a convenient alias
    await use(page);
  },
});

export { expect } from '@playwright/test';
