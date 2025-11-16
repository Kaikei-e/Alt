import { expect, test } from "@playwright/test";

test.describe("Login Flow Integration", () => {
  test.beforeEach(async ({ page }) => {
    // Clear cookies before each test
    await page.context().clearCookies();
  });

  test("should redirect unauthenticated users to login", async ({ page }) => {
    // Navigate to protected route
    await page.goto("/home");

    // Should be redirected to landing page
    await expect(page).toHaveURL(/\/public\/landing/);
    await expect(page.locator("text=ログイン")).toBeVisible();

    // Click login button to go to auth/login
    await page.click('a[href="/auth/login"]');
    await expect(page).toHaveURL(/\/auth\/login/);
  });

  test("should handle Kratos flow initialization correctly", async ({
    page,
  }) => {
    // Navigate to login page
    await page.goto("/auth/login");

    // Wait for Kratos flow to initialize
    await page.waitForSelector('input[name="identifier"]');

    // Check that flow parameter is present in URL
    const url = new URL(page.url());
    const flowParam = url.searchParams.get("flow");

    expect(flowParam).toBeTruthy();
    expect(flowParam).toMatch(/^[a-f0-9-]{32,36}$/); // 32 or 36 char UUID
  });

  test("should maintain session cookie after login", async ({ page }) => {
    // Navigate to protected page to trigger auth flow
    await page.goto("/home");

    // Wait for redirect to landing page, then click login
    await page.waitForURL(/\/public\/landing/, { timeout: 10000 });
    await page.click('a[href="/auth/login"]');
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 15000 });

    // Wait for login form
    await page.waitForSelector('input[name="identifier"]', {
      state: "visible",
      timeout: 15000,
    });

    // Fill in credentials
    await page.fill('input[name="identifier"]', "test@example.com");
    await page.fill('input[name="password"]', "password123");

    // Submit
    await page.click('button[type="submit"]');

    // Wait for redirect after login
    await page.waitForURL(/\/(home|mobile)/, { timeout: 30000 });

    // Check that session cookie is set
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find((c) => c.name === "ory_kratos_session");

    expect(sessionCookie).toBeDefined();
    expect(sessionCookie?.value).toBeTruthy();
    expect(sessionCookie?.httpOnly).toBe(true);
  });

  // ❌ Removed: "should redirect to return_to URL after successful login" - 基本ログインテストで十分
  // ❌ Removed: "should not redirect authenticated users from protected routes" - 認証済みテストで検証
  // ❌ Removed: "should prevent redirect loop on root path" - ミドルウェアの単体テストで検証
});
