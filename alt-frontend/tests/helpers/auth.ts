import { Page, expect } from "@playwright/test";
import { LoginPage, DesktopPage } from "../pages";

/**
 * Login helper function that performs the complete login flow
 * @deprecated Use LoginPage.performLogin() instead
 */
export async function loginUser(
  page: Page,
  email = "test@example.com",
  password = "password123",
) {
  const loginPage = new LoginPage(page);

  // Access a protected route to trigger auth flow
  await page.goto("/desktop/home");

  // Wait for auth redirect to mock server
  await page.waitForURL(/localhost:4545.*login\/browser/, { timeout: 10000 });

  // Mock server should redirect back with flow
  await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 10000 });

  // Use page object for login
  await loginPage.performLogin(email, password);
}

/**
 * Quick login using Page Object Model
 */
export async function quickLogin(
  page: Page,
  email = "test@example.com",
  password = "password123",
) {
  const loginPage = new LoginPage(page);
  await loginPage.performLogin(email, password);
}

/**
 * Wait for authentication state to be ready
 */
export async function waitForAuthState(page: Page) {
  await page.waitForLoadState("networkidle");
}

/**
 * Verify user is authenticated and not redirected to login
 */
export async function verifyAuthenticated(page: Page) {
  const desktopPage = new DesktopPage(page);
  await desktopPage.waitForAuthenticated();
}

/**
 * Test fixture for authenticated page context
 */
export async function withAuthenticatedContext(
  page: Page,
  testFn: () => Promise<void>,
) {
  await verifyAuthenticated(page);
  await testFn();
}
