import { test, expect, type Route } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";

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

test.describe("mobile feeds routes - viewed", () => {
	test("viewed page shows empty history state", async ({ page }) => {
		await page.route("**/api/v1/feeds/fetch/viewed/cursor**", (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);

		await gotoMobileRoute(page, "feeds/viewed");

		await expect(page.getByText("No History Yet")).toBeVisible();
		await expect(page.getByTestId("empty-viewed-feeds-icon")).toBeVisible();
	});
});
