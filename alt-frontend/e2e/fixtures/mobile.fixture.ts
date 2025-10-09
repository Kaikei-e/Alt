import { test as base, devices } from '@playwright/test';
import type { Page } from '@playwright/test';

/**
 * Mobile fixture type
 */
export type MobileFixture = {
  mobilePage: Page;
};

/**
 * Supported mobile device configurations
 */
export const MOBILE_DEVICES = {
  iphone13: devices['iPhone 13'],
  iphone13Pro: devices['iPhone 13 Pro'],
  iphoneSE: devices['iPhone SE'],
  pixel5: devices['Pixel 5'],
  galaxyS9: devices['Galaxy S9+'],
} as const;

/**
 * Mobile viewport sizes for custom testing
 */
export const MOBILE_VIEWPORTS = {
  iphoneSE: { width: 375, height: 667 },
  iphone13: { width: 390, height: 844 },
  iphone13ProMax: { width: 428, height: 926 },
  pixel5: { width: 393, height: 851 },
} as const;

/**
 * Test fixture for mobile scenarios
 * Provides a page with mobile device configuration (iPhone 13 by default)
 */
export const test = base.extend<MobileFixture>({
  mobilePage: async ({ browser }, use) => {
    // Create a new context with iPhone 13 configuration
    const context = await browser.newContext({
      ...MOBILE_DEVICES.iphone13,
    });

    const page = await context.newPage();

    await use(page);

    // Cleanup
    await context.close();
  },
});

/**
 * Create a custom mobile fixture with specific device
 */
export function createMobileFixture(
  deviceName: keyof typeof MOBILE_DEVICES
) {
  return base.extend<MobileFixture>({
    mobilePage: async ({ browser }, use) => {
      const context = await browser.newContext({
        ...MOBILE_DEVICES[deviceName],
      });

      const page = await context.newPage();

      await use(page);

      await context.close();
    },
  });
}

export { expect } from '@playwright/test';
