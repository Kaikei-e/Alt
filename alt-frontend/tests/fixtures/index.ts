import { test as base, expect } from '@playwright/test';
import { LoginPage, DesktopPage } from '../pages';

/**
 * Test fixtures with page objects
 */
export const test = base.extend<{
  loginPage: LoginPage;
  desktopPage: DesktopPage;
}>({
  loginPage: async ({ page }, use) => {
    const loginPage = new LoginPage(page);
    await use(loginPage);
  },
  
  desktopPage: async ({ page }, use) => {
    const desktopPage = new DesktopPage(page);
    await use(desktopPage);
  },
});

export { expect } from '@playwright/test';