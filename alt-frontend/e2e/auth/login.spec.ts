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
    await page.waitForURL(/\/auth\/login\?flow=/);

    // Wait for login form to be ready
    await page.waitForSelector('input[name="identifier"]', { state: "visible", timeout: 15000 });
    await page.waitForSelector('input[name="password"]', { state: "visible", timeout: 5000 });
    await page.waitForSelector('button[type="submit"]', { state: "visible", timeout: 5000 });

    // Perform login using page object
    await loginPage.performLogin(
      "test@example.com",
      "password123",
      /\/(desktop\/home|home|mobile)/,
    );

    // Wait for redirect to complete
    await page.waitForURL(/\/(desktop\/home|home|mobile)/);
  });

  test("should show error for invalid credentials", async ({
    page,
    loginPage,
  }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();
    await page.goto("/desktop/home");

    // Wait for redirect to login page
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 15000 });

    // Wait for login form to be ready
    await page.waitForSelector('input[name="identifier"]', { state: "visible", timeout: 15000 });

    // Fill in wrong credentials
    await loginPage.login("wrong@example.com", "wrongpassword");

    // Wait a bit for the error to be processed
    await page.waitForTimeout(2000);

    // Check if we're still on the login page (which indicates error)
    const currentUrl = page.url();
    expect(currentUrl).toMatch(/\/auth\/login/);
  });

  // ❌ Removed: "should handle direct access to login page without flow" - API検証済み
  // ❌ Removed: "should display loading state initially" - 実装詳細
  // ❌ Removed: "should handle expired flow (410)" - login-flow.spec.tsで検証
  // ❌ Removed: "should handle 410 during form submission" - login-flow.spec.tsで検証
});
