import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_FEEDS_WITHOUT_ARTICLE_ID,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
} from "../../fixtures/mockData";

// Swipe mode uses Connect-RPC for data fetching (SSR disabled)
const SWIPE_FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			title: "AI Trends",
			description: "Latest AI updates across the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
	],
	nextCursor: "next-cursor-123",
	hasMore: true,
};

const VIEWED_FEEDS_EMPTY = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

test.describe("mobile feeds routes - swipe", () => {
	test("swipe page renders swipe card and action footer", async ({ page }) => {
		// Mock Connect-RPC endpoints (used by +page.ts loader and client-side components)
		// Note: SSR is disabled for this page, all data is fetched client-side via Connect-RPC
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, SWIPE_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/swipe");

		await expect(page.getByTestId("swipe-card")).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		await expect(page.getByTestId("action-footer")).toBeVisible();
	});

	test("swipe marks feed as read even without articleId (404 article)", async ({
		page,
	}) => {
		// Use mock data without articleId (simulates 404 article)
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_WITHOUT_ARTICLE_ID),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		// Simulate 404 error when fetching article content
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillError(route, "Article not found", 404),
		);

		// Track markAsRead API call
		let markAsReadCalled = false;
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
			markAsReadCalled = true;
			return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
		});

		await gotoMobileRoute(page, "feeds/swipe");
		await expect(page.getByTestId("swipe-card")).toBeVisible();

		// Perform swipe left (dismiss)
		const card = page.getByTestId("swipe-card");
		const box = await card.boundingBox();
		if (!box) throw new Error("Card not found");

		await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
		await page.mouse.down();
		await page.mouse.move(box.x - 200, box.y + box.height / 2, { steps: 10 });
		await page.mouse.up();

		// Verify markAsRead was called even though articleId is empty
		await expect.poll(() => markAsReadCalled, { timeout: 5000 }).toBe(true);
	});
});
