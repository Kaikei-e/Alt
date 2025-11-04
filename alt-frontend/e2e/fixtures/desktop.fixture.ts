import type { Page } from "@playwright/test";
import { test as base, devices } from "@playwright/test";

/**
 * Desktop fixture type
 */
export type DesktopFixture = {
  desktopPage: Page;
};

/**
 * Desktop viewport sizes for testing different resolutions
 */
export const DESKTOP_VIEWPORTS = {
  hd: { width: 1366, height: 768 },
  fullHd: { width: 1920, height: 1080 },
  ultraWide: { width: 2560, height: 1440 },
} as const;

/**
 * Test fixture for desktop scenarios
 * Provides a page with desktop device configuration
 */
export const test = base.extend<DesktopFixture>({
  desktopPage: async ({ browser }, use) => {
    // Create a new context with desktop Chrome configuration
    const context = await browser.newContext({
      ...devices["Desktop Chrome"],
      viewport: DESKTOP_VIEWPORTS.fullHd,
    });

    const page = await context.newPage();

    await use(page);

    // Cleanup
    await context.close();
  },
});

export { expect } from "@playwright/test";
