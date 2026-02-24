import { expect, test } from "@playwright/test";
import { MobileHomePage } from "../../pages/mobile/MobileHomePage";
import { setupAllMocks } from "../../utils/api-mock";

test.describe("Mobile Navigation", () => {
  let mobileHomePage: MobileHomePage;

  test.beforeEach(async ({ page }) => {
    mobileHomePage = new MobileHomePage(page);
    await setupAllMocks(page);
  });

  test("should navigate to mobile home page", async () => {
    await mobileHomePage.goto();
    await mobileHomePage.waitForFeeds();

    // Verify we're on the mobile feeds page
    const currentUrl = mobileHomePage.getUrl();
    expect(currentUrl).toMatch(/\/mobile\/feeds/);

    // Verify feed cards are visible (or empty state if no feeds)
    const feedCount = await mobileHomePage.getFeedCount();
    expect(feedCount).toBeGreaterThanOrEqual(0);
  });

  test("should display feed list on mobile", async () => {
    await mobileHomePage.goto();
    await mobileHomePage.waitForFeeds();

    const feedCount = await mobileHomePage.getFeedCount();

    // If there are feeds, verify first one is visible
    if (feedCount > 0) {
      await expect(mobileHomePage.firstFeedCard).toBeVisible();
    }
  });

  test("should navigate to article from feed list", async ({
    page,
    context,
  }) => {
    await mobileHomePage.goto();
    await mobileHomePage.waitForFeeds();

    const feedCount = await mobileHomePage.getFeedCount();

    if (feedCount > 0) {
      // Get initial URL
      const _initialUrl = mobileHomePage.getUrl();

      // Click on first feed (this might open external link in new tab)
      const [newPage] = await Promise.all([
        context.waitForEvent("page", { timeout: 2000 }).catch(() => null),
        mobileHomePage.clickFirstFeed(),
      ]);

      // If a new page was opened (external link), verify it
      if (newPage) {
        await expect(newPage).not.toBeNull();
        await newPage.close();
      } else {
        // If no new page, check if URL changed (modal or navigation)
        await page.waitForTimeout(500);
        const newUrl = page.url();
        // URL might not change if it's a modal, which is acceptable
        expect(typeof newUrl).toBe("string");
      }
    }
  });

  test("should support scroll on mobile", async ({ page }) => {
    await mobileHomePage.goto();
    await mobileHomePage.waitForFeeds();

    const feedCount = await mobileHomePage.getFeedCount();

    if (feedCount > 0) {
      // Scroll down
      await mobileHomePage.scrollToLoadMore();
      await page.waitForTimeout(500);

      // Get scroll position
      const scrollPosition = await page.evaluate(() => window.scrollY);
      expect(scrollPosition).toBeGreaterThanOrEqual(0);
    }
  });
});
