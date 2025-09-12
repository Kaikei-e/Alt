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

    // Verify login form elements
    await loginPage.verifyLoginPageElements();

    // Perform login using page object
    await loginPage.performLogin(
      "test@example.com",
      "password123",
      "/desktop/home",
    );
  });

  test("should show error for invalid credentials", async ({
    page,
    loginPage,
  }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();
    await page.goto("/auth/login?flow=test-flow-id");

    await waitForPageReady(page, { waitForSelector: "form", timeout: 10000 });
    await loginPage.login("wrong@example.com", "wrongpassword");

    // Check for error message
    const errorText = await loginPage.waitForError();
    expect(errorText).toMatch(/credentials are invalid/i);
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

    // Check for loading state using page object
    const isLoading = await loginPage.isLoading();
    expect(isLoading).toBe(true);
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

    // Should redirect to new flow with preserved return_to
    await expect(page).toHaveURL(/\/auth\/login\?flow=.*/, { timeout: 25000 });
  });
});
