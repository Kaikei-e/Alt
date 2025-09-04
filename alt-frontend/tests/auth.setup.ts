import { test as setup, expect } from '@playwright/test';
import { LoginPage } from './pages/LoginPage';

const authFile = 'playwright/.auth/user.json';

setup('authenticate', async ({ page }) => {
  const loginPage = new LoginPage(page);
  
  // Access a protected route to trigger auth flow
  await page.goto('/desktop/home');
  
  // Wait for auth redirect to mock server
  await page.waitForURL(/localhost:4545.*login\/browser/, { timeout: 10000 });
  
  // Mock server should redirect back with flow
  await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 10000 });
  
  // Use page object for login
  await loginPage.performLogin('test@example.com', 'password123', '/desktop/home');
  
  // Save authentication state
  await page.context().storageState({ path: authFile });
});