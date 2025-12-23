import { test, expect, type Route } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";

const FEEDS_RESPONSE = {
  data: [
    {
      title: "AI Trends",
      description: "Latest AI updates across the ecosystem.",
      link: "https://example.com/ai-trends",
      published: "2025-12-20T10:00:00Z",
      author: { name: "Alice" },
    },
    {
      title: "Svelte 5 Tips",
      description: "Runes-first patterns for fast interfaces.",
      link: "https://example.com/svelte-5",
      published: "2025-12-19T09:00:00Z",
      author: { name: "Bob" },
    },
  ],
  next_cursor: null,
  has_more: false,
};

const VIEWED_FEEDS_EMPTY = {
  data: [],
  next_cursor: null,
  has_more: false,
};

const fulfillJson = async (
  route: Route,
  body: unknown,
  status: number = 200,
) => {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
};

test.describe("mobile feeds routes", () => {
  test("feeds list renders and supports mark-as-read", async ({ page }) => {
    let readPayload: { feed_url?: string } | null = null;

    // Use more generic pattern to catch potential base path variations
    // The previous pattern might have been too specific or failed in some environments
    await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
      fulfillJson(route, FEEDS_RESPONSE),
    );
    await page.route("**/api/v1/feeds/fetch/viewed/cursor**", (route) =>
      fulfillJson(route, VIEWED_FEEDS_EMPTY),
    );
    await page.route("**/api/v1/feeds/read", async (route) => {
      try {
        readPayload = route.request().postDataJSON() as { feed_url?: string };
      } catch {
        readPayload = null;
      }
      await fulfillJson(route, { ok: true });
    });

    await gotoMobileRoute(page, "feeds");

    const cards = page.getByTestId("feed-card");
    await expect(cards).toHaveCount(2);

    const firstCard = page.getByRole("article", {
      name: /Feed: AI Trends/i,
    });
    await expect(firstCard).toBeVisible();

    await firstCard.getByRole("button", { name: /mark as read/i }).click();

    await expect(firstCard).toHaveCount(0);
    await expect.poll(() => readPayload?.feed_url).toBe(
      "https://example.com/ai-trends",
    );
  });
});
