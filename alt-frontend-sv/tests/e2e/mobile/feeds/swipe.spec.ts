import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";

// Swipe mode uses view: "swipe" parameter which returns single item
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

// v1 REST API format for swipe +page.ts loader
const V1_FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			url: "https://example.com/ai-trends",
			title: "AI Trends",
			description: "Latest AI updates across the ecosystem.",
			link: "https://example.com/ai-trends",
			published_at: "2025-12-20T10:00:00Z",
			tags: ["AI", "Tech"],
			author: { name: "Alice" },
			thumbnail: null,
			feed_domain: "example.com",
			read_at: null,
			created_at: new Date().toISOString(),
			updated_at: new Date().toISOString(),
		},
	],
	next_cursor: "next-cursor-123",
	has_more: true,
};

const V1_ARTICLE_CONTENT = {
	content: "<p>This is a mocked article content.</p>",
};

test.describe("mobile feeds routes - swipe", () => {
	test("swipe page renders swipe card and action footer", async ({ page }) => {
		// Mock v1 REST API endpoints (used by +page.ts loader)
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, V1_FEEDS_RESPONSE),
		);
		await page.route("**/api/v1/articles/content**", (route) =>
			fulfillJson(route, V1_ARTICLE_CONTENT),
		);

		// Mock Connect-RPC endpoints (used by client-side components)
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, SWIPE_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		// Initial load might fail SSR if mocks are not hit by server, but client should retry or load
		await gotoMobileRoute(page, "feeds/swipe");

		await expect(page.getByTestId("swipe-card")).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		await expect(page.getByTestId("action-footer")).toBeVisible();
	});
});
