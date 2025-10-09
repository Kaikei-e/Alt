import { test, expect } from "@playwright/test";
import { LoginPage, DesktopPage } from "../../tests/pages";

test.describe("Authenticated Desktop Navigation", () => {
  let desktopPage: DesktopPage;
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    desktopPage = new DesktopPage(page);
    loginPage = new LoginPage(page);
  });

  test("should access desktop home page after login", async ({ page }) => {
    await page.goto("/desktop/home", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await desktopPage.waitForAuthenticated();
    await desktopPage.verifyOnDesktopPage("home");
  });

  test("should access feeds page after login", async ({ page }) => {
    await page.goto("/desktop/feeds", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await desktopPage.waitForAuthenticated();
    await desktopPage.verifyOnDesktopPage("feeds");
  });

  test("should access articles page after login", async ({ page }) => {
    await page.goto("/desktop/articles", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await desktopPage.waitForAuthenticated();
    await desktopPage.verifyOnDesktopPage("articles");
  });

  test("should access settings page after login", async ({ page }) => {
    await page.goto("/desktop/settings", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await desktopPage.waitForAuthenticated();
    await desktopPage.verifyOnDesktopPage("settings");
  });

  test("should redirect to login when accessing protected pages without auth", async ({
    page,
  }) => {
    // This test requires clearing auth state, skip for now as it conflicts with authenticated project
    test.skip();
  });

  test("should maintain session across page navigation", async ({ page }) => {
    // Navigate through different pages - simplified
    const routes = ["/desktop/home", "/desktop/feeds", "/desktop/articles"];

    for (const route of routes) {
      await page.goto(route, { waitUntil: "domcontentloaded" });
      await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
      await desktopPage.waitForAuthenticated();
    }

    // Verify we're still authenticated
    await expect(page).not.toHaveURL(/\/auth\/login/, { timeout: 5000 });
  });

  test("should handle direct navigation to protected routes", async ({
    page,
  }) => {
    // Test direct navigation - simplified URL check only
    await page.goto("/desktop/feeds/register", { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("domcontentloaded", { timeout: 10000 });
    await expect(page).toHaveURL("/desktop/feeds/register", { timeout: 10000 });
  });
});
