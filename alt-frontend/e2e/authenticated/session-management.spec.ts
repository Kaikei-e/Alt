import { test, expect } from "@playwright/test";
import { LoginPage, DesktopPage } from "../../tests/pages";

test.describe("Session Management", () => {
  let loginPage: LoginPage;
  let desktopPage: DesktopPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    desktopPage = new DesktopPage(page);
  });

  test("should maintain session after browser refresh", async ({ page }) => {
    // Navigate to a protected page (should be authenticated via setup)
    await page.goto("/home");
    await page.waitForURL(/\/home/);

    // Refresh the page
    await page.reload();

    // Should still be on the same page, not redirected to login
    await page.waitForURL(/\/home/);
    await expect(page).not.toHaveURL(/\/auth\/login/);
  });

  test("should handle session cookies correctly", async ({ page }) => {
    // Navigate to authenticated page (should be authenticated via setup)
    await page.goto("/home");
    await page.waitForURL(/\/home/);

    // Check that session cookie is set
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(
      (cookie) => cookie.name === "ory_kratos_session",
    );
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
      await page.goto("/desktop/home");
      await page.waitForURL(/\/public\/landing/, { timeout: 10000 });
    } finally {
      await context.close();
    }
  });

  test("should protect all desktop routes", async ({ browser }) => {
    // Create a new context without authentication
    const context = await browser.newContext();
    const page = await context.newPage();

    try {
      const protectedRoutes = [
        "/desktop/home",
        "/desktop/feeds",
        "/desktop/articles",
        "/desktop/settings",
        "/desktop/feeds/register",
        "/desktop/articles/search",
      ];

      for (const route of protectedRoutes) {
        await page.goto(route);
        await page.waitForURL(/\/public\/landing/, { timeout: 10000 });
      }
    } finally {
      await context.close();
    }
  });

  test("should preserve return_to parameter for protected routes", async ({
    browser,
  }) => {
    // Create a new context without authentication
    const context = await browser.newContext();
    const page = await context.newPage();

    try {
      const newLoginPage = new LoginPage(page);

      // Try to access a protected page directly
      await page.goto("/desktop/settings");

      // Should redirect to landing with return_to parameter
      await page.waitForURL(/\/public\/landing/, { timeout: 15000 });

      // Verify return_to is in the URL
      const url = page.url();
      expect(url).toContain("return_to");
      expect(url).toContain("desktop%2Fsettings");

      // Click login button to go to auth/login
      await page.click('a[href="/auth/login"]');
      await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 15000 });

      // Now log in using page object
      await newLoginPage.performLogin(
        "test@example.com",
        "password123",
        "/desktop/settings",
      );
    } finally {
      await context.close();
    }
  });

  test("should handle concurrent sessions correctly", async ({ browser }) => {
    // Create two contexts (like two browser windows)
    const context1 = await browser.newContext();
    const context2 = await browser.newContext();

    const page1 = await context1.newPage();
    const page2 = await context2.newPage();

    try {
      const loginPage1 = new LoginPage(page1);
      const loginPage2 = new LoginPage(page2);
      const desktopPage1 = new DesktopPage(page1);
      const desktopPage2 = new DesktopPage(page2);

      // Log in on first page
      await page1.goto("/desktop/home");
      await page1.waitForURL(/\/public\/landing/);
      await page1.click('a[href="/auth/login"]');
      await page1.waitForURL(/\/auth\/login\?flow=/);
      await loginPage1.performLogin(
        "test@example.com",
        "password123",
        "/desktop/home",
      );

      // Second page should still require login
      await desktopPage2.verifyProtectedRouteRedirect("/desktop/home");

      // Log in on second page too
      await loginPage2.performLogin(
        "test@example.com",
        "password123",
        "/desktop/feeds",
      );

      // Both sessions should remain valid
      await page1.reload();
      await desktopPage1.verifyOnDesktopPage("home");

      await page2.reload();
      await desktopPage2.verifyOnDesktopPage("feeds");
    } finally {
      await context1.close();
      await context2.close();
    }
  });
});
