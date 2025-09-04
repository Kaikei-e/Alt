import { test as setup, expect } from '@playwright/test';
import { LoginPage } from './pages/LoginPage';

const authFile = 'playwright/.auth/user.json';

setup('authenticate', async ({ page }) => {
  const loginPage = new LoginPage(page);

  // Access a protected route to trigger auth flow
  await page.goto('/desktop/home');

  // Wait for auth redirect to mock server (port is dynamic on CI)
  const mockPort = process.env.PW_MOCK_PORT || '4545';
  const re = new RegExp(`localhost:${mockPort}.*login\\/browser`);
  await page.waitForURL(re, { timeout: 15000 });

  // Mock server should redirect back with flow
  await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 15000 });

  // Use page object for login
  await loginPage.performLogin('test@example.com', 'password123', '/desktop/home');

  // Save authentication state
  await page.context().storageState({ path: authFile });
});