import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("mobile feeds routes", () => {
	test("feeds list renders with multiple cards", async ({ page }) => {
		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds");

		// Wait for loading to complete - check for loading indicator to disappear
		const loadingIndicator = page.getByText("Loading more...");
		await expect(loadingIndicator).not.toBeVisible({ timeout: 10000 });

		// Verify feed cards are rendered (mock returns 2 feeds)
		const cards = page.getByTestId("feed-card");
		const cardCount = await cards.count();
		expect(cardCount).toBeGreaterThan(0);

		// Check first feed card
		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Verify it has the expected content
		await expect(firstCard.getByText("Deep dive into the ecosystem")).toBeVisible();
		await expect(firstCard.getByText("by Alice")).toBeVisible();

		// Verify mark as read button exists and is clickable
		const markAsReadButton = firstCard.getByRole("button", { name: /mark .* as read/i });
		await expect(markAsReadButton).toBeVisible();
		await expect(markAsReadButton).toBeEnabled();
	});

	test("feed card has external link", async ({ page }) => {
		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds");

		// Wait for cards to load
		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Check the external link
		const externalLink = firstCard.getByRole("link", { name: /Open AI Trends in external link/i });
		await expect(externalLink).toBeVisible();
		await expect(externalLink).toHaveAttribute("href", "https://example.com/ai-trends");
		await expect(externalLink).toHaveAttribute("target", "_blank");
	});

	test("feed card has show details button", async ({ page }) => {
		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds");

		// Wait for cards to load
		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Verify show details button exists
		const detailsButton = firstCard.getByRole("button", { name: /show details/i });
		await expect(detailsButton).toBeVisible();
	});
});
