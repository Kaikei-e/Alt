import { expect, test } from "@playwright/test";
import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { DesktopFeedsPage } from "../../pages/desktop/DesktopFeedsPage";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * Regression: Visual Preview OG image fallback on mark-as-read.
 *
 * Root cause: the OG image proxy rate-limits per upstream host (1 req/s). A grid
 * of same-host images plus the mark-as-read burst saturates it, returning 429.
 * The old card pinned `imageError=true` on the first <img> onerror, so a
 * *transient* rate-limit became a *permanent* fallback gradient — including for
 * cards that were still resolving when the user marked an article as read.
 *
 * Fix contract (this spec is the executable acceptance test):
 *  - a transiently rate-limited OG image is retried and ends up displayed, not
 *    collapsed to the fallback gradient;
 *  - marking a feed as read never turns a surviving card's image into a fallback.
 */

// 1x1 transparent PNG.
const PNG_1x1 = Buffer.from(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	"base64",
);

/** Build an image-proxy URL whose base64 segment decodes to a real upstream host. */
function proxyUrl(original: string): string {
	return `/v1/images/proxy/testsig/${Buffer.from(original).toString("base64")}`;
}

const FEEDS_WITH_OG = {
	data: [
		{
			id: "feed-1",
			articleId: "article-1",
			title: "AI Trends",
			description: "Deep dive into the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
			ogImageProxyUrl: proxyUrl("https://img.example.com/ai.png"),
		},
		{
			id: "feed-2",
			articleId: "article-2",
			title: "Svelte 5 Tips",
			description: "Runes-first patterns for fast interfaces.",
			link: "https://example.com/svelte-5",
			published: "1 day ago",
			createdAt: new Date().toISOString(),
			author: "Bob",
			ogImageProxyUrl: proxyUrl("https://img.example.com/svelte.png"),
		},
	],
	nextCursor: "",
	hasMore: false,
};

test.describe("Visual Preview — OG image resilience", () => {
	test("retries a rate-limited OG image and shows it instead of the fallback", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (r) =>
			fulfillJson(r, FEEDS_WITH_OG),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (r) =>
			fulfillJson(r, FEEDS_WITH_OG),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (r) =>
			fulfillJson(r, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		// First request per URL -> 429 (rate limited), every subsequent -> 200 image.
		const hits = new Map<string, number>();
		await page.route("**/v1/images/proxy/**", async (route) => {
			const url = route.request().url();
			const n = (hits.get(url) ?? 0) + 1;
			hits.set(url, n);
			if (n === 1) {
				await route.fulfill({ status: 429, body: "" });
			} else {
				await route.fulfill({
					status: 200,
					contentType: "image/png",
					body: PNG_1x1,
				});
			}
		});

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		const card = feedsPage.getFeedCardByTitle("AI Trends");
		await expect(card).toBeVisible({ timeout: 15000 });

		// After one 429 + retry, the image element appears and the fallback never sticks.
		await expect(card.locator('[data-testid="card-image"]')).toBeVisible({
			timeout: 15000,
		});
		await expect(card.locator('[data-testid="image-fallback"]')).toHaveCount(0);
	});

	test("marking a feed as read keeps the surviving card's image (no fallback regression)", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (r) =>
			fulfillJson(r, FEEDS_WITH_OG),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (r) =>
			fulfillJson(r, FEEDS_WITH_OG),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (r) =>
			fulfillJson(r, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.markAsRead, (r) =>
			fulfillJson(r, CONNECT_MARK_AS_READ_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (r) =>
			fulfillJson(r, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
		await page.route("**/v1/images/proxy/**", (route) =>
			route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
		);

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		const survivor = feedsPage.getFeedCardByTitle("Svelte 5 Tips");
		await expect(survivor).toBeVisible({ timeout: 15000 });
		await expect(survivor.locator('[data-testid="card-image"]')).toBeVisible({
			timeout: 15000,
		});

		// Open the first card and mark it as read.
		await feedsPage.selectFeed("AI Trends");
		await feedsPage.markAsReadButton.click();

		// The surviving card's image must remain an image, never the fallback gradient.
		await expect(survivor.locator('[data-testid="card-image"]')).toBeVisible();
		await expect(
			survivor.locator('[data-testid="image-fallback"]'),
		).toHaveCount(0);
	});
});
