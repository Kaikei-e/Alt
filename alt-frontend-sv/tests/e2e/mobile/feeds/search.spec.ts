import { expect, type Route, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";

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

test.describe("mobile feeds routes - search", () => {
	test("search page shows results for a valid query", async ({ page }) => {
		await page.route("**/api/v1/feeds/search", (route) =>
			fulfillJson(route, SEARCH_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/search");

		await page.getByTestId("search-input").fill("AI");
		await page.getByRole("button", { name: "Search" }).click();

		const results = page.getByTestId("search-result-item");
		await expect(results).toHaveCount(1);
		await expect(page.getByRole("link", { name: "AI Weekly" })).toBeVisible();
		await expect(page.getByText("Search Results (1)")).toBeVisible();
	});

	test("search page shows validation errors on short queries", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/search");

		await page.getByTestId("search-input").fill("A");
		await page.getByRole("button", { name: "Search" }).click();

		await expect(
			page.getByText("Search query must be at least 2 characters"),
		).toBeVisible();
	});
});
