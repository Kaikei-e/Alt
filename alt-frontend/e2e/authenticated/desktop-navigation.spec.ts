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
    // Use page object for navigation
    await desktopPage.navigateToHome();
    await page.waitForLoadState("domcontentloaded");
    await desktopPage.verifyOnDesktopPage("home");
  });

  test("should access feeds page after login", async ({ page }) => {
    await desktopPage.navigateToFeeds();
    await desktopPage.verifyOnDesktopPage("feeds");
  });

  test("should access articles page after login", async ({ page }) => {
    await desktopPage.navigateToArticles();
    await desktopPage.verifyOnDesktopPage("articles");
  });

  test("should access settings page after login", async ({ page }) => {
    await desktopPage.navigateToSettings();
    await desktopPage.verifyOnDesktopPage("settings");
  });

  test("should redirect to login when accessing protected pages without auth", async ({
    page,
  }) => {
    await desktopPage.verifyProtectedRouteRedirect("/desktop/home");
  });

  test("should maintain session across page navigation", async ({ page }) => {
    // Test navigation using page object
    await desktopPage.testNavigation();
  });

  test("should handle direct navigation to protected routes", async ({
    page,
  }) => {
    // Test direct navigation with browser address bar simulation
    await desktopPage.navigateToFeedsRegister();
    await expect(page).toHaveURL("/desktop/feeds/register");

    // Should not redirect to login since user is authenticated
    await desktopPage.waitForAuthenticated();
  });
});
