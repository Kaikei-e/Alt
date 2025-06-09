import { Feed } from "@/schema/feed";
import { expect, test } from "@playwright/test";

const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}. This is a longer description to test how the UI handles different text lengths.`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String(index + 1).padStart(2, "0")}T12:00:00Z`,
  }));
};

test("FeedCard", async ({ page }) => {
  // Mock the feeds API endpoints before navigating to the page
  const feeds = generateMockFeeds(10, 1);

  await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(feeds),
    });
  });

  // Also mock the fallback endpoint (getAllFeeds)
  await page.route("**/api/v1/feeds/fetch/list", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(feeds),
    });
  });

  // Mock the read status endpoint
  await page.route("**/api/v1/feeds/read", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ message: "Feed marked as read" }),
    });
  });

  await page.goto("/mobile/feeds");

  // Test the mock data structure (these are synchronous assertions)
  expect(feeds.length).toBe(10);
  expect(feeds[0].title).toBe("Test Feed 1");
  expect(feeds[0].description).toBe(
    "Description for test feed 1. This is a longer description to test how the UI handles different text lengths.",
  );
  expect(feeds[0].link).toBe("https://example.com/feed1");
  expect(feeds[0].published).toBe("2024-01-01T12:00:00Z");

  expect(feeds[5].id).toBe("6");
  expect(feeds[5].title).toBe("Test Feed 6");
  expect(feeds[5].description).toBe(
    "Description for test feed 6. This is a longer description to test how the UI handles different text lengths.",
  );
  expect(feeds[5].link).toBe("https://example.com/feed6");
  expect(feeds[5].published).toBe("2024-01-06T12:00:00Z");

  // check the feed card is visible - use more specific selectors to avoid strict mode violations
  await expect(page.getByRole('link', { name: 'Test Feed 1', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Test Feed 6', exact: true })).toBeVisible();

  // check the feed card is not visible
  await expect(page.getByRole('link', { name: 'Test Feed 11', exact: true })).not.toBeVisible();
});
