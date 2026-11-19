import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

const SUBSCRIPTIONS_EMPTY = { subscriptions: [] };
const LIST_SUBSCRIPTIONS =
	"**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions";
const GET_ARTICLE_SOURCE_URL =
	"**/api/v2/alt.articles.v2.ArticleService/GetArticleSourceURL";

const GALLERY_SCROLL_FEEDS = {
	data: Array.from({ length: 16 }, (_, index) => ({
		id: `feed-${index + 1}`,
		articleId: `article-${index + 1}`,
		title: `Gallery Story ${index + 1}`,
		description: `Description for story ${index + 1}.`,
		link: `https://example.com/story-${index + 1}`,
		published: "2 hours ago",
		createdAt: new Date().toISOString(),
		author: "Author",
	})),
	nextCursor: "",
	hasMore: false,
};

test.describe("mobile feeds — visual preview gallery", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(LIST_SUBSCRIPTIONS, (route) =>
			fulfillJson(route, SUBSCRIPTIONS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: "https://example.com/ai-trends",
				content: "<p>Mocked gallery article content.</p>",
				articleId: "article-1",
			}),
		);
	});

	test("renders the feeds as thumbnail tiles instead of the swipe hand-off", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/visual-preview");

		const tiles = page.getByTestId("gallery-tile");
		await expect(tiles).toHaveCount(2);
		await expect(tiles.first()).toBeVisible();

		await expect(page.getByText("AI Trends")).toBeVisible();
		await expect(page.getByText("Svelte 5 Tips")).toBeVisible();

		await expect(
			page.getByRole("link", { name: /swipe visual preview/i }),
		).toHaveCount(0);
	});

	test("lays the gallery out in two columns at phone width", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/visual-preview");
		await expect(page.getByTestId("gallery-tile").first()).toBeVisible();

		const columns = await page
			.getByTestId("gallery-grid")
			.evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(" "));

		expect(columns).toHaveLength(2);
	});

	test("opens reader overlay at /articles/<id>, Back restores list scroll, and reload renders standalone page", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, GALLERY_SCROLL_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, GALLERY_SCROLL_FEEDS),
		);
		await page.route(GET_ARTICLE_SOURCE_URL, (route) =>
			fulfillJson(route, {
				sourceUrl: "https://example.com/story-12",
				title: "Gallery Story 12",
			}),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: "https://example.com/story-12",
				content: "<p>Story 12 full body.</p>",
				articleId: "article-12",
			}),
		);

		await gotoMobileRoute(page, "feeds/visual-preview");
		await expect(page.getByTestId("gallery-tile")).toHaveCount(16);

		// Scroll down so the window scroll position is non-zero before opening.
		const targetTile = page.getByRole("button", {
			name: "Open Gallery Story 12",
		});
		await targetTile.scrollIntoViewIfNeeded();
		const scrollBeforeOpen = await page.evaluate(() => window.scrollY);
		expect(scrollBeforeOpen).toBeGreaterThan(0);

		await targetTile.click();

		const overlay = page.getByTestId("reader-overlay");
		await expect(overlay).toBeVisible();
		await expect(
			overlay.getByRole("heading", { name: "Gallery Story 12" }),
		).toBeVisible();
		await expect(overlay.getByText("Story 12 full body.")).toBeVisible();
		await expect
			.poll(() => new URL(page.url()).pathname)
			.toBe("/articles/article-12");

		// Browser Back closes the overlay and restores the underlying list scroll.
		await page.goBack();
		await expect(overlay).toBeHidden();
		await expect
			.poll(() => new URL(page.url()).pathname)
			.toBe("/feeds/visual-preview");
		const scrollAfterBack = await page.evaluate(() => window.scrollY);
		expect(Math.abs(scrollAfterBack - scrollBeforeOpen)).toBeLessThanOrEqual(4);

		// Re-open and reload on /articles/article-12 -> renders standalone page.
		await targetTile.click();
		await expect
			.poll(() => new URL(page.url()).pathname)
			.toBe("/articles/article-12");

		await page.reload();
		await expect(page.getByTestId("reader-overlay")).toHaveCount(0);
		await expect(
			page.getByRole("heading", { name: "Gallery Story 12" }),
		).toBeVisible({ timeout: 15000 });
		await expect(page.getByText("Story 12 full body.")).toBeVisible();
	});
});
