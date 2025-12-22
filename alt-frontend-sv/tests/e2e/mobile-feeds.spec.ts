import { test, expect, type Route } from "@playwright/test";

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

const SEARCH_RESPONSE = {
  data: [
    {
      title: "AI Weekly",
      description:
        "A deep dive into AI research, tooling, and production learnings.",
      link: "https://example.com/ai-weekly",
      published: "2025-12-18T08:30:00Z",
      author: { name: "Casey" },
    },
  ],
  next_cursor: null,
  has_more: false,
};

const STATS_RESPONSE = {
  feed_amount: { amount: 12 },
  total_articles: { amount: 345 },
  unsummarized_articles: { amount: 7 },
};

const UNREAD_RESPONSE = {
  count: 42,
};

const ARTICLE_CONTENT_RESPONSE = {
  content: "<p>This is a mocked article.</p>",
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

    await page.route("**/sv/api/v1/feeds/fetch/cursor**", (route) =>
      fulfillJson(route, FEEDS_RESPONSE),
    );
    await page.route("**/sv/api/v1/feeds/fetch/viewed/cursor**", (route) =>
      fulfillJson(route, VIEWED_FEEDS_EMPTY),
    );
    await page.route("**/sv/api/v1/feeds/read", async (route) => {
      try {
        readPayload = route.request().postDataJSON() as { feed_url?: string };
      } catch {
        readPayload = null;
      }
      await fulfillJson(route, { ok: true });
    });

    await page.goto("/mobile/feeds");

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

  test("search page shows results for a valid query", async ({ page }) => {
    await page.route("**/sv/api/v1/feeds/search", (route) =>
      fulfillJson(route, SEARCH_RESPONSE),
    );

    await page.goto("/mobile/feeds/search");

    await page.getByTestId("search-input").fill("AI");
    await page.getByRole("button", { name: "Search" }).click();

    const results = page.getByTestId("search-result-item");
    await expect(results).toHaveCount(1);
    await expect(page.getByRole("link", { name: "AI Weekly" })).toBeVisible();
    await expect(page.getByText("Search Results (1)")).toBeVisible();
  });

  test("search page shows validation errors on short queries", async ({ page }) => {
    await page.goto("/mobile/feeds/search");

    await page.getByTestId("search-input").fill("A");
    await page.getByRole("button", { name: "Search" }).click();

    await expect(
      page.getByText("Search query must be at least 2 characters"),
    ).toBeVisible();
  });

  test("viewed page shows empty history state", async ({ page }) => {
    await page.route("**/sv/api/v1/feeds/fetch/viewed/cursor**", (route) =>
      fulfillJson(route, VIEWED_FEEDS_EMPTY),
    );

    await page.goto("/mobile/feeds/viewed");

    await expect(page.getByText("No History Yet")).toBeVisible();
    await expect(page.getByTestId("empty-viewed-feeds-icon")).toBeVisible();
  });

  test("stats page renders mocked counters", async ({ page }) => {
    await page.addInitScript(() => {
      class MockEventSource {
        url: string;
        readyState = 1;
        onopen: ((this: EventSource, ev: Event) => void) | null = null;
        onmessage: ((this: EventSource, ev: MessageEvent) => void) | null = null;
        onerror: ((this: EventSource, ev: Event) => void) | null = null;

        constructor(url: string) {
          this.url = url;
          setTimeout(() => {
            this.onopen?.(new Event("open"));
          }, 0);
        }

        close() {
          this.readyState = 2;
        }

        addEventListener() {}
        removeEventListener() {}
        dispatchEvent() {
          return false;
        }
      }

      // @ts-expect-error - override EventSource for E2E stability.
      window.EventSource = MockEventSource;
    });

    await page.route("**/sv/api/v1/feeds/stats/detailed", (route) =>
      fulfillJson(route, STATS_RESPONSE),
    );
    await page.route("**/sv/api/v1/feeds/count/unreads", (route) =>
      fulfillJson(route, UNREAD_RESPONSE),
    );

    await page.goto("/mobile/feeds/stats");

    await expect(page.getByText("Total Feeds")).toBeVisible();
    await expect(page.getByText("12")).toBeVisible();
    await expect(page.getByText("345")).toBeVisible();
    await expect(page.getByText("7")).toBeVisible();
    await expect(page.getByText("Today's Unread")).toBeVisible();
    await expect(page.getByText("42")).toBeVisible();
  });

  test("manage page can open the add feed form", async ({ page }) => {
    await page.goto("/mobile/feeds/manage");

    await expect(page.getByText("Feed Management")).toBeVisible();

    await page.getByRole("button", { name: "Add a new feed" }).click();
    await expect(
      page.getByPlaceholder("https://example.com/feed.xml"),
    ).toBeVisible();

    await page.getByRole("button", { name: "Add feed" }).click();
    await expect(page.getByText("Please enter the RSS URL.")).toBeVisible();
  });

  test("swipe page renders swipe card and action footer", async ({ page }) => {
    await page.route("**/sv/api/v1/feeds/fetch/cursor**", (route) =>
      fulfillJson(route, FEEDS_RESPONSE),
    );
    await page.route("**/sv/api/v1/feeds/fetch/viewed/cursor**", (route) =>
      fulfillJson(route, VIEWED_FEEDS_EMPTY),
    );
    await page.route("**/sv/api/v1/articles/content**", (route) =>
      fulfillJson(route, ARTICLE_CONTENT_RESPONSE),
    );

    await page.goto("/mobile/feeds/swipe");

    await expect(page.getByTestId("swipe-card")).toBeVisible();
    await expect(page.getByRole("heading", { name: "AI Trends" })).toBeVisible();
    await expect(page.getByTestId("action-footer")).toBeVisible();
  });
});
