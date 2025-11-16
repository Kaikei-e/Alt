import { expect, test } from "../../tests/fixtures";

test.describe("Login Flow", () => {
  test("should complete full login flow", async ({ page, loginPage }) => {
    // Clear any existing session cookies
    await page.context().clearCookies();

    // Access protected route to trigger auth flow
    await page.goto("/desktop/home");
    await page.waitForLoadState("domcontentloaded");

    // Should redirect to landing page first
    await page.waitForURL(/\/public\/landing/, { timeout: 10000 });
    await page.click('a[href="/auth/login"]');
    await page.waitForURL(/\/auth\/login\?flow=/);

    // Wait for login form to be ready
    await page.waitForSelector('input[name="identifier"]', {
      state: "visible",
      timeout: 15000,
    });
    await page.waitForSelector('input[name="password"]', {
      state: "visible",
      timeout: 5000,
    });
    await page.waitForSelector('button[type="submit"]', {
      state: "visible",
      timeout: 5000,
    });

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

    // Wait for redirect to landing page, then click login
    await page.waitForURL(/\/public\/landing/, { timeout: 10000 });
    await page.click('a[href="/auth/login"]');
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 15000 });

    // Wait for login form to be ready
    await page.waitForSelector('input[name="identifier"]', {
      state: "visible",
      timeout: 15000,
    });

    // Fill in wrong credentials
    await loginPage.login("wrong@example.com", "wrongpassword");

    // Wait for error state - either error message or staying on login page
    await Promise.race([
      page
        .locator('[data-testid="error-message"]')
        .waitFor({ state: "visible", timeout: 5000 }),
      page
        .locator('[role="alert"]')
        .waitFor({ state: "visible", timeout: 5000 }),
      page.waitForLoadState("networkidle", { timeout: 5000 }),
    ]).catch(() => {});

    // Check if we're still on the login page (which indicates error)
    const currentUrl = page.url();
    expect(currentUrl).toMatch(/\/auth\/login/);
  });

  // ❌ Removed: "should handle direct access to login page without flow" - API検証済み
  // ❌ Removed: "should display loading state initially" - 実装詳細
  // ❌ Removed: "should handle expired flow (410)" - login-flow.spec.tsで検証
  // ❌ Removed: "should handle 410 during form submission" - login-flow.spec.tsで検証
});
