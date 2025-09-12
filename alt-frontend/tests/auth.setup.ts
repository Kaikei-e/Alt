import { test as setup, expect } from "@playwright/test";
import { LoginPage } from "./pages/LoginPage";

const authFile = "playwright/.auth/user.json";

setup("authenticate", async ({ page }) => {
  const loginPage = new LoginPage(page);

  // Enable debug logging for auth setup
  console.log("[AUTH-SETUP] Starting authentication setup");

  let retryCount = 0;
  const maxRetries = 3;

  while (retryCount < maxRetries) {
    try {
      console.log(`[AUTH-SETUP] Attempt ${retryCount + 1}/${maxRetries}`);

      // Clear cookies before each attempt
      await page.context().clearCookies();

      // Access a protected route to trigger auth flow
      await page.goto("/desktop/home");
      await page.waitForLoadState("domcontentloaded");
      console.log("[AUTH-SETUP] Navigated to protected route");

      // Wait for auth redirect to mock server (port is dynamic on CI)
      const mockPort = process.env.PW_MOCK_PORT || "4545";
      const re = new RegExp(`localhost:${mockPort}.*login\\/browser`);
      await page.waitForURL(re, { timeout: 25000 });
      console.log("[AUTH-SETUP] Redirected to mock auth server");

      // Mock server should redirect back with flow
      await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 25000 });
      console.log("[AUTH-SETUP] Redirected back with flow");

      // Use page object for login
      await loginPage.performLogin(
        "test@example.com",
        "password123",
        "/desktop/home",
      );
      console.log("[AUTH-SETUP] Login completed successfully");

      // Save authentication state
      await page.context().storageState({ path: authFile });
      console.log("[AUTH-SETUP] Authentication state saved");

      // If we reach here, authentication was successful
      break;
    } catch (error) {
      retryCount++;
      console.log(`[AUTH-SETUP] Attempt ${retryCount} failed:`, error.message);

      if (retryCount >= maxRetries) {
        console.log("[AUTH-SETUP] All retry attempts failed");
        throw error;
      }

      // Wait before retrying
      await page.waitForTimeout(2000);
    }
  }
});
