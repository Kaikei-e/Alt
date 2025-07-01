import { expect, test } from "@playwright/test";
import { mockApiEndpoints } from "../helpers/mockApi";
import { Feed } from "@/schema/feed";

const feeds: Feed[] = [
  {
    id: "1",
    title: "Short title",
    description: "desc",
    link: "https://example.com/short",
    published: "2024-01-01T00:00:00Z",
  },
  {
    id: "2",
    title: "Very long title that spans multiple lines and should not affect the icon size at all",
    description: "desc",
    link: "https://example.com/long",
    published: "2024-01-02T00:00:00Z",
  },
];

test.describe("FeedCard Icon Size", () => {
  test.beforeEach(async ({ page }) => {
    await page.unrouteAll();
    await mockApiEndpoints(page, { feeds });
  });

  test("icon size remains consistent for varying title lengths", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");
    const icons = page.locator('[data-testid^="feed-link-icon-"]');
    const firstIcon = icons.nth(0);
    const secondIcon = icons.nth(1);

    await expect(firstIcon).toBeVisible({ timeout: 10000 });
    await expect(secondIcon).toBeVisible({ timeout: 10000 });

    const firstSize = await firstIcon.evaluate((el) => ({
      width: el.clientWidth,
      height: el.clientHeight,
    }));
    const secondSize = await secondIcon.evaluate((el) => ({
      width: el.clientWidth,
      height: el.clientHeight,
    }));

    expect(firstSize.width).toBe(secondSize.width);
    expect(firstSize.height).toBe(secondSize.height);
  });
});
