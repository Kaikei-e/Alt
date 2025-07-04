import { Page } from "@playwright/test";
import { Feed, BackendFeedItem } from "@/schema/feed";
import { Article } from "@/schema/article";

export const mockApiEndpoints = async (
  page: Page,
  data: {
    feeds?: Feed[];
    articles?: Article[];
    healthStatus?: { status: string };
  },
) => {
  const { feeds = [], articles = [], healthStatus = { status: "ok" } } = data;

  // Health check endpoints
  await page.route("**/api/v1/health", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(healthStatus),
    });
  });

  await page.route("http://localhost/api/v1/health", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(healthStatus),
    });
  });

  await page.route("http://localhost:8080/api/v1/health", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(healthStatus),
    });
  });

  // Feeds endpoints
  if (feeds.length > 0) {
    // Convert Feed[] to BackendFeedItem[] for API compatibility
    const backendFeeds: BackendFeedItem[] = feeds.map((feed) => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock cursor-based feeds endpoint (NEW)
    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      const url = new URL(route.request().url());
      const cursor = url.searchParams.get("cursor");
      const limit = parseInt(url.searchParams.get("limit") || "20");

      // Simple pagination simulation
      const startIndex = cursor ? parseInt(cursor) : 0;
      const endIndex = Math.min(startIndex + limit, backendFeeds.length);
      const paginatedFeeds = backendFeeds.slice(startIndex, endIndex);
      const nextCursor =
        endIndex < backendFeeds.length ? endIndex.toString() : null;

      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: paginatedFeeds,
          next_cursor: nextCursor,
        }),
      });
    });

    await page.route(
      "http://localhost/api/v1/feeds/fetch/cursor**",
      async (route) => {
        const url = new URL(route.request().url());
        const cursor = url.searchParams.get("cursor");
        const limit = parseInt(url.searchParams.get("limit") || "20");

        // Simple pagination simulation
        const startIndex = cursor ? parseInt(cursor) : 0;
        const endIndex = Math.min(startIndex + limit, backendFeeds.length);
        const paginatedFeeds = backendFeeds.slice(startIndex, endIndex);
        const nextCursor =
          endIndex < backendFeeds.length ? endIndex.toString() : null;

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: paginatedFeeds,
            next_cursor: nextCursor,
          }),
        });
      },
    );

    await page.route(
      "http://localhost:8080/api/v1/feeds/fetch/cursor**",
      async (route) => {
        const url = new URL(route.request().url());
        const cursor = url.searchParams.get("cursor");
        const limit = parseInt(url.searchParams.get("limit") || "20");

        const startIndex = cursor ? parseInt(cursor) : 0;
        const endIndex = Math.min(startIndex + limit, backendFeeds.length);
        const paginatedFeeds = backendFeeds.slice(startIndex, endIndex);
        const nextCursor =
          endIndex < backendFeeds.length ? endIndex.toString() : null;

        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            data: paginatedFeeds,
            next_cursor: nextCursor,
          }),
        });
      },
    );

    // Mock paginated feeds endpoint (LEGACY - keeping for backward compatibility)
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route(
      "http://localhost/api/v1/feeds/fetch/page/0",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(backendFeeds),
        });
      },
    );

    await page.route(
      "http://localhost:8080/api/v1/feeds/fetch/page/0",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(backendFeeds),
        });
      },
    );

    // Mock all feeds endpoint
    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route(
      "http://localhost/api/v1/feeds/fetch/list",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(backendFeeds),
        });
      },
    );

    await page.route(
      "http://localhost:8080/api/v1/feeds/fetch/list",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(backendFeeds),
        });
      },
    );

    // Mock feed read status endpoint - correct endpoint
    await page.route("**/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed read status updated" }),
      });
    });

    await page.route("http://localhost/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed read status updated" }),
      });
    });

    await page.route(
      "http://localhost:8080/api/v1/feeds/read",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ message: "Feed read status updated" }),
        });
      },
    );

    // Legacy endpoint for backward compatibility
    await page.route("**/api/v1/feeds/viewed", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.route("http://localhost/api/v1/feeds/viewed", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.route(
      "http://localhost:8080/api/v1/feeds/viewed",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ message: "Feed marked as read" }),
        });
      },
    );

    // Mock feed status endpoint (additional endpoint that might be used)
    await page.route("**/api/v1/feeds/status", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed status updated" }),
      });
    });

    await page.route("http://localhost/api/v1/feeds/status", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed status updated" }),
      });
    });

    await page.route(
      "http://localhost:8080/api/v1/feeds/status",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ message: "Feed status updated" }),
        });
      },
    );

    // Mock feed details endpoint
    await page.route("**/api/v1/feeds/fetch/details", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_url: "https://example.com/feed1",
          summary: "Test summary for this feed",
        }),
      });
    });

    await page.route(
      "http://localhost/api/v1/feeds/fetch/details",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test summary for this feed",
          }),
        });
      },
    );

    await page.route(
      "http://localhost:8080/api/v1/feeds/fetch/details",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            feed_url: "https://example.com/feed1",
            summary: "Test summary for this feed",
          }),
        });
      },
    );
  }

  // Articles endpoints
  if (articles.length > 0) {
    await page.route("**/api/v1/articles/search**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(articles),
      });
    });

    await page.route(
      "http://localhost/api/v1/articles/search**",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(articles),
        });
      },
    );

    await page.route(
      "http://localhost:8080/api/v1/articles/search**",
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(articles),
        });
      },
    );
  }
};

export const generateMockFeeds = (
  count: number,
  startId: number = 1,
): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `Description for test feed ${startId + index}. This is a longer description to test how the UI handles different text lengths.`,
    link: `https://example.com/feed${startId + index}`,
    published: `2024-01-${String(index + 1).padStart(2, "0")}T12:00:00Z`,
  }));
};

export const generateMockArticles = (
  count: number,
  startId: number = 1,
): Article[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Article ${startId + index}`,
    content: `Content for test article ${startId + index}. This is a longer content to test how the UI handles different text lengths.`,
  }));
};
