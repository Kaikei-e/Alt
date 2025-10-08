import { test, expect } from "@playwright/test";

test.describe("Login Flow Integration", () => {
  test.beforeEach(async ({ page }) => {
    // Clear cookies before each test
    await page.context().clearCookies();
  });

  test("should redirect unauthenticated users to login", async ({ page }) => {
    // Navigate to protected route
    await page.goto("/home");

    // Should be redirected to login page
    await expect(page).toHaveURL(/\/auth\/login/);
    await expect(page.locator("text=ログイン")).toBeVisible();
  });

  test("should redirect to return_to URL after successful login", async ({
    page,
  }) => {
    const returnTo = "/mobile/feeds";

    // Navigate to protected route
    await page.goto(returnTo);

    // Should be redirected to login with return_to parameter
    await expect(page).toHaveURL(
      new RegExp(`/auth/login.*return_to=${encodeURIComponent(returnTo)}`),
    );

    // Wait for login form to load
    await page.waitForSelector('input[name="identifier"]', { timeout: 10000 });

    // Fill in login credentials
    await page.fill('input[name="identifier"]', "test@example.com");
    await page.fill('input[name="password"]', "test-password");

    // Submit login form
    await page.click('button[type="submit"]');

    // After successful login, should redirect to original return_to URL
    await expect(page).toHaveURL(returnTo, { timeout: 15000 });
  });

  test("should maintain session cookie after login", async ({ page }) => {
    // Navigate to login page
    await page.goto("/auth/login");

    // Wait for login form
    await page.waitForSelector('input[name="identifier"]', { timeout: 10000 });

    // Fill in credentials
    await page.fill('input[name="identifier"]', "test@example.com");
    await page.fill('input[name="password"]', "test-password");

    // Submit
    await page.click('button[type="submit"]');

    // Wait for redirect after login
    await page.waitForURL(/\/(home|mobile)/, { timeout: 15000 });

    // Check that session cookie is set
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find((c) => c.name === "ory_kratos_session");

    expect(sessionCookie).toBeDefined();
    expect(sessionCookie?.value).toBeTruthy();
    expect(sessionCookie?.httpOnly).toBe(true);
  });

  test("should not redirect authenticated users from protected routes", async ({
    page,
  }) => {
    // First, log in
    await page.goto("/auth/login");
    await page.waitForSelector('input[name="identifier"]', { timeout: 10000 });
    await page.fill('input[name="identifier"]', "test@example.com");
    await page.fill('input[name="password"]', "test-password");
    await page.click('button[type="submit"]');

    // Wait for redirect after login
    await page.waitForURL(/\/(home|mobile)/, { timeout: 15000 });

    // Navigate to another protected route
    await page.goto("/mobile/feeds");

    // Should remain on the protected route, not redirect to login
    await expect(page).toHaveURL("/mobile/feeds");
    await expect(page.locator("text=ログイン")).not.toBeVisible();
  });

  test("should handle Kratos flow initialization correctly", async ({
    page,
  }) => {
    // Navigate to login page
    await page.goto("/auth/login");

    // Wait for Kratos flow to initialize
    await page.waitForSelector('input[name="identifier"]', { timeout: 10000 });

    // Check that flow parameter is present in URL
    const url = new URL(page.url());
    const flowParam = url.searchParams.get("flow");

    expect(flowParam).toBeTruthy();
    expect(flowParam).toMatch(/^[a-f0-9-]{36}$/); // UUID format
  });

  test("should prevent redirect loop on root path", async ({ page }) => {
    // Navigate to root
    await page.goto("/");

    // Should eventually redirect to login (for unauthenticated users)
    // or home (for authenticated users), but not loop infinitely
    await page.waitForURL(/\/(auth\/login|home|mobile)/, { timeout: 10000 });

    // Verify we're not in a redirect loop by checking we stayed on the page
    await page.waitForTimeout(2000); // Wait a bit
    const finalUrl = page.url();

    expect(finalUrl).toMatch(/\/(auth\/login|home|mobile)/);
  });
});
