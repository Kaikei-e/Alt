import { expect, test } from "@playwright/test";

test.describe("Session Management", () => {
  const baseURL =
    process.env.PLAYWRIGHT_BASE_URL ?? `http://localhost:${process.env.PW_APP_PORT || "3010"}`;

  test("should maintain session after browser refresh", async ({ page }) => {
    // Navigate to a protected page (should be authenticated via setup)
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 30000 });
    await page.waitForURL(/\/home/, { timeout: 30000 });

    // Refresh the page - use networkidle to avoid NS_ERROR_ABORT in Firefox
    try {
      await page.reload({ waitUntil: "networkidle", timeout: 15000 });
    } catch (e) {
      // Fallback to domcontentloaded if networkidle times out
      await page.reload({ waitUntil: "domcontentloaded", timeout: 15000 });
    }

    // Should still be on the same page, not redirected to login
    await page.waitForURL(/\/home/, { timeout: 15000 });
    await expect(page).not.toHaveURL(/\/auth\/login/);
  });

  test("should handle session cookies correctly", async ({ page }) => {
    // Navigate to authenticated page (should be authenticated via setup)
    await page.goto("/home", { waitUntil: "domcontentloaded", timeout: 30000 });
    await page.waitForURL(/\/home/, { timeout: 30000 });

    // Check that session cookie is set
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find((cookie) => cookie.name === "ory_kratos_session");
    expect(sessionCookie).toBeDefined();
    expect(sessionCookie?.httpOnly).toBe(true);
  });

  test("should handle invalid session gracefully", async ({ browser }) => {
    // Create a new context without authentication
    const context = await browser.newContext();
    const page = await context.newPage();

    try {
      // Set an invalid session cookie
      await context.addCookies([
        {
          name: "ory_kratos_session",
          value: "invalid-session-id",
          domain: "localhost",
          path: "/",
          httpOnly: true,
          secure: false,
          sameSite: "Lax",
        },
      ]);

      // Try to access a protected page - should redirect to landing
      await page.goto("/desktop/home", { waitUntil: "domcontentloaded", timeout: 30000 });
      await page.waitForURL(/\/public\/landing/, { timeout: 30000 });
    } finally {
      await context.close();
    }
  });

  test("should preserve return_to parameter for protected routes", async ({ browser }) => {
    // Create a new context without authentication
    const context = await browser.newContext({ baseURL });
    const page = await context.newPage();

    try {
      // Try to access protected route without auth
      const targetRoute = "/desktop/feeds";

      // Navigate and wait for redirect to landing page
      await page.goto(targetRoute, { waitUntil: "networkidle", timeout: 30000 }).catch(() => {});
      await page.waitForURL(/\/public\/landing/, { timeout: 15000 });

      const landingUrl = new URL(page.url());
      expect(landingUrl.pathname).toBe("/public/landing");
      expect(landingUrl.searchParams.get("return_to")).toBe("/desktop/feeds");

      // Use CTA to proceed to login page so the return_to is preserved
      const loginLink = page.getByRole("link", { name: "ログイン" });
      await expect(loginLink).toBeVisible({ timeout: 10000 });
      await loginLink.click();

      await page.waitForURL(/\/auth\/login/, { timeout: 15000 });
      const loginUrl = new URL(page.url());
      expect(loginUrl.pathname).toBe("/auth/login");
      expect(loginUrl.searchParams.get("return_to")).toBe("/desktop/feeds");
    } finally {
      await context.close();
    }
  });

  test("should handle concurrent sessions correctly", async ({ browser }) => {
    // Create two different browser contexts using the authenticated storage state
    const context1 = await browser.newContext({ storageState: "playwright/.auth/user.json" });
    const context2 = await browser.newContext({ storageState: "playwright/.auth/user.json" });

    const page1 = await context1.newPage();
    const page2 = await context2.newPage();

    try {
      // Both pages should be able to access protected routes
      await page1.goto("/desktop/home", { waitUntil: "domcontentloaded", timeout: 15000 });
      await page2.goto("/desktop/feeds", { waitUntil: "domcontentloaded", timeout: 15000 });

      // Both should maintain their sessions
      await expect(page1).toHaveURL(/\/desktop\/home/, { timeout: 10000 });
      await expect(page2).toHaveURL(/\/desktop\/feeds/, { timeout: 10000 });

      // Verify both have session cookies
      const cookies1 = await context1.cookies();
      const cookies2 = await context2.cookies();

      const session1 = cookies1.find((c) => c.name === "ory_kratos_session");
      const session2 = cookies2.find((c) => c.name === "ory_kratos_session");

      expect(session1).toBeDefined();
      expect(session2).toBeDefined();
    } finally {
      await context1.close();
      await context2.close();
    }
  });
});
