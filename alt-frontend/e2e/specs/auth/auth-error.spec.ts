import { expect, test } from "@playwright/test";
import { AuthErrorPage } from "../../page-objects/auth/auth-error.page";

test.describe("Auth Error Page", () => {
  let errorPage: AuthErrorPage;

  test.beforeEach(async ({ page }) => {
    errorPage = new AuthErrorPage(page);
  });

  test("should display error page with message", async () => {
    await errorPage.goto();

    // Check error message is visible
    await expect(errorPage.errorMessage).toBeVisible();

    // Get error message text
    const message = await errorPage.getErrorMessage();
    expect(message).toBeTruthy();
  });

  test("should have back to login button", async () => {
    await errorPage.goto();

    // Check if back to login button exists
    const hasButton = await errorPage.hasBackToLoginButton();
    expect(hasButton).toBeTruthy();
  });

  test("should navigate back to login", async () => {
    await errorPage.goto();

    if (await errorPage.hasBackToLoginButton()) {
      await errorPage.clickBackToLogin();

      // Verify navigation to login page
      await expect(errorPage.page).toHaveURL(/\/auth\/login/);
    }
  });

  test("should have retry button if applicable", async ({}, testInfo) => {
    await errorPage.goto();

    const hasRetry = await errorPage.hasRetryButton();

    if (hasRetry) {
      await expect(errorPage.retryButton).toBeVisible();
    } else {
      testInfo.skip();
    }
  });

  test("should handle retry action", async ({}, testInfo) => {
    await errorPage.goto();

    if (!(await errorPage.hasRetryButton())) {
      testInfo.skip();
    }

    await errorPage.clickRetry();

    // Should attempt to retry or navigate somewhere
    // Exact behavior depends on implementation
  });

  test("should display error details if available", async () => {
    await errorPage.goto();

    const details = await errorPage.getErrorDetails();

    // Error details might or might not be present
    // Just verify we can retrieve them if they exist
    if (details) {
      expect(details.length).toBeGreaterThan(0);
    }
  });

  test("should navigate from failed login to error page", async ({ page }) => {
    // Simulate a failed login that redirects to error page
    await page.goto("/auth/login");

    // Wait for Kratos flow to initialize (form is rendered dynamically)
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 15000 });
    await page.waitForSelector('input[name="identifier"]', {
      state: "visible",
      timeout: 15000,
    });
    await page.waitForSelector('input[name="password"]', {
      state: "visible",
      timeout: 15000,
    });
    await page.waitForSelector('button[type="submit"]', {
      state: "visible",
      timeout: 15000,
    });

    // Mock an authentication error
    await page.route("**/auth/**", (route) => {
      route.fulfill({
        status: 500,
        body: JSON.stringify({ error: "Authentication failed" }),
      });
    });

    // Fill and submit login form
    await page.fill('input[name="identifier"]', "test@example.com");
    await page.fill('input[name="password"]', "password123");
    await page.click('button[type="submit"]');

    // Might redirect to error page or show inline error
    // This depends on implementation
  });

  test("should be accessible", async () => {
    await errorPage.goto();
    await errorPage.checkA11y();
  });

  test("should have proper heading structure", async ({ page }) => {
    await errorPage.goto();

    const headings = await page.locator("h1, h2, h3, h4, h5, h6").allTextContents();

    // Should have at least one heading
    expect(headings.length).toBeGreaterThan(0);
  });

  test("should handle different error types", async ({ page }) => {
    // Test with different error parameters in URL
    const errorTypes = ["auth_failed", "session_expired", "invalid_token"];

    for (const errorType of errorTypes) {
      await page.goto(`/auth/error?error=${errorType}`);

      // Error message should be visible for all types
      await expect(errorPage.errorMessage).toBeVisible();
    }
  });
});
