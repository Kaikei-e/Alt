import { expect, type Route, test } from "@playwright/test";
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

test.describe("mobile feeds routes - swipe", () => {
	test("swipe page renders swipe card and action footer", async ({ page }) => {
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, FEEDS_RESPONSE),
		);
		await page.route("**/api/v1/feeds/fetch/viewed/cursor**", (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);
		await page.route("**/api/v1/articles/content**", (route) =>
			fulfillJson(route, ARTICLE_CONTENT_RESPONSE),
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
