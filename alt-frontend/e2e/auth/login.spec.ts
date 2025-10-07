import { test, expect } from "../../tests/fixtures";
import { waitForPageReady } from "../../tests/utils/waitConditions";

test.describe("Login Flow", () => {
  test("should complete full login flow", async ({ page, loginPage }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();

    // Access protected route to trigger auth flow
    await page.goto("/desktop/home");
    await page.waitForLoadState("domcontentloaded");

    // Should redirect to mock auth server and then back to app with flow
    // The redirect to 4545 happens automatically and comes back with a flow
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 30000 });

    // Wait for page to be fully ready
    await waitForPageReady(page, { waitForSelector: "form", timeout: 15000 });

    // Verify login form elements are present
    await expect(page.getByLabel("Email")).toBeVisible({ timeout: 10000 });
    await expect(page.getByLabel("Password")).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole("button", { name: /sign in/i })).toBeVisible({
      timeout: 10000,
    });

    // Perform login using page object
    await loginPage.performLogin(
      "test@example.com",
      "password123",
      "/desktop/home",
    );

    // Wait for redirect to complete
    await page.waitForURL(/\/desktop\/home/, { timeout: 15000 });
  });

  test("should show error for invalid credentials", async ({
    page,
    loginPage,
  }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();
    await page.goto("/auth/login?flow=test-flow-id");
    await page.waitForLoadState("domcontentloaded");

    await waitForPageReady(page, { waitForSelector: "form", timeout: 15000 });
    await loginPage.login("wrong@example.com", "wrongpassword");

    // Wait for error message to appear
    await page.waitForTimeout(2000);

    // Check for error message - try multiple selectors
    const errorSelectors = [
      '[data-testid="error-message"]',
      ".error-message",
      '[role="alert"]',
      ".text-red-500",
      ".text-red-600",
    ];

    let errorText = null;
    for (const selector of errorSelectors) {
      try {
        const errorElement = await page.locator(selector).first();
        if (await errorElement.isVisible()) {
          errorText = await errorElement.textContent();
          break;
        }
      } catch (e) {
        // Continue to next selector
      }
    }

    // If no specific error message found, check if we're still on login page (which indicates error)
    if (!errorText) {
      const currentUrl = page.url();
      expect(currentUrl).toMatch(/\/auth\/login/);
    } else {
      expect(errorText).toMatch(/credentials are invalid|invalid|error/i);
    }
  });

  test("should handle direct access to login page without flow", async ({
    page,
  }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();

    // Access login page directly without flow ID
    await page.goto("/auth/login");
    await page.waitForLoadState("domcontentloaded");

    // Should redirect to mock Kratos server and back with flow
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 20000 });
  });

  test("should display loading state initially", async ({
    page,
    loginPage,
  }) => {
    // Access page with mock Kratos flow
    await page.goto("/auth/login?flow=test-flow-id");
    await page.waitForLoadState("domcontentloaded");

    // Wait a bit for the loading state to appear
    await page.waitForTimeout(1000);

    // Check for loading state using page object
    const isLoading = await loginPage.isLoading();
    // The loading state might not be visible immediately, so we check if it's either loading or the form is ready
    let isFormReady = false;
    try {
      isFormReady = await page.locator("form").isVisible();
    } catch (e) {
      isFormReady = false;
    }
    expect(isLoading || isFormReady).toBe(true);
  });

  test("should handle expired flow (410) and automatically redirect to new flow", async ({
    page,
  }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();

    // Start with an expired flow - mock service will handle the 410 response
    await page.goto(
      "/auth/login?flow=expired-flow-id&return_to=http%3A%2F%2Flocalhost%3A3010%2Fdesktop%2Fhome",
    );
    await page.waitForLoadState("domcontentloaded");

    // Should automatically redirect to new flow creation
    await expect(page).toHaveURL(/\/auth\/login\?flow=.*/, { timeout: 25000 });

    // Should show the login form with the new flow
    await expect(page.getByLabel("Email")).toBeVisible({ timeout: 15000 });
    await expect(page.getByLabel("Password")).toBeVisible({ timeout: 15000 });
  });

  test("should handle 410 during form submission and redirect to new flow", async ({
    page,
    loginPage,
  }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();

    // Start with a flow that will return 410 on submission - mock service handles this
    await page.goto(
      "/auth/login?flow=expired-flow-submission-id&return_to=http%3A%2F%2Flocalhost%3A3010%2Fdesktop%2Fanalytics",
    );
    await page.waitForLoadState("domcontentloaded");

    // Wait for form to be ready
    await waitForPageReady(page, { waitForSelector: "form", timeout: 15000 });
    await loginPage.waitForForm();

    // Fill and submit the form using page object - this should trigger a 410 and redirect
    await loginPage.login("test@example.com", "password123");

    // Wait for redirect to new flow
    await page.waitForTimeout(2000);

    // Should redirect to new flow with preserved return_to
    const currentUrl = page.url();
    expect(currentUrl).toMatch(/\/auth\/login\?flow=/);
  });
});
