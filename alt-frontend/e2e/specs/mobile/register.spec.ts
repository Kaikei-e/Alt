import { test, expect } from "@playwright/test";
import { MobileRegisterFeedPage } from "../../pages/mobile/MobileRegisterFeedPage";
import { setupAllMocks } from "../../utils/api-mock";

test.describe("Mobile Register Feed", () => {
  let registerPage: MobileRegisterFeedPage;

  test.beforeEach(async ({ page }) => {
    registerPage = new MobileRegisterFeedPage(page);
    await setupAllMocks(page);

    // Mock feed registration API
    await page.route("**/v1/feeds/register", async (route) => {
      const request = route.request();
      if (request.method() === "POST") {
        const body = request.postDataJSON();
        if (body?.feed_url?.includes("invalid")) {
          await route.fulfill({
            status: 400,
            contentType: "application/json",
            body: JSON.stringify({ error: "Invalid feed URL" }),
          });
        } else {
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({ message: "Feed registered successfully" }),
          });
        }
      } else {
        await route.continue();
      }
    });
  });

  test("should display register feed page", async () => {
    await registerPage.goto();
    await registerPage.waitForReady();

    await expect(registerPage.registerFeedHeading).toBeVisible();
    await expect(registerPage.registerFeedHeading).toHaveText(
      "Register RSS Feed",
    );
  });

  test("should display feed URL input and register button", async () => {
    await registerPage.goto();
    await registerPage.waitForReady();

    await expect(registerPage.feedUrlInput).toBeVisible();
    await expect(registerPage.registerButton).toBeVisible();
  });

  test("should show validation error for invalid URL", async () => {
    await registerPage.goto();
    await registerPage.waitForReady();

    await registerPage.enterFeedUrl("not-a-valid-url");

    // Wait for validation
    await registerPage.page.waitForTimeout(500);

    const hasError = await registerPage.hasValidationError();
    expect(hasError).toBe(true);
  });

  test("should accept valid feed URL", async () => {
    await registerPage.goto();
    await registerPage.waitForReady();

    await registerPage.enterFeedUrl("https://example.com/feed.xml");

    // Wait for validation
    await registerPage.page.waitForTimeout(500);

    const hasError = await registerPage.hasValidationError();
    expect(hasError).toBe(false);
  });

  test("should clear input when cleared", async () => {
    await registerPage.goto();
    await registerPage.waitForReady();

    await registerPage.enterFeedUrl("https://example.com/feed.xml");
    await registerPage.clearInput();

    const value = await registerPage.feedUrlInput.inputValue();
    expect(value).toBe("");
  });
});
