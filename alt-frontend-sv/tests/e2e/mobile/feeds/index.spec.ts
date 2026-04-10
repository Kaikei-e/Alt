import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("mobile feeds routes", () => {
	test("feeds list renders with multiple cards", async ({
		page,
		mobileFeedsPage,
	}) => {
		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await mobileFeedsPage.goto();

		// Wait for feed cards to render (auto-waits through SystemLoader)
		const cards = page.getByTestId("feed-card-container");
		await expect(cards.first()).toBeVisible({ timeout: 10000 });
		const cardCount = await cards.count();
		expect(cardCount).toBeGreaterThan(0);

		// Check first feed card
		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Verify it has the expected content
		await expect(
			firstCard.getByText("Deep dive into the ecosystem"),
		).toBeVisible();

		// Verify mark as read button exists and is clickable
		const markAsReadButton = firstCard.getByRole("button", {
			name: /mark .* as read/i,
		});
		await expect(markAsReadButton).toBeVisible();
		await expect(markAsReadButton).toBeEnabled();
	});

	test("feed card has external link", async ({ page, mobileFeedsPage }) => {
		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await mobileFeedsPage.goto();

		// Wait for cards to load
		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Title link serves as external link in redesigned card
		const externalLink = firstCard.getByRole("link", {
			name: /AI Trends/i,
		});
		await expect(externalLink).toBeVisible();
		await expect(externalLink).toHaveAttribute("target", "_blank");
	});

	test("feed card has show details button", async ({
		page,
		mobileFeedsPage,
	}) => {
		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await mobileFeedsPage.goto();

		// Wait for cards to load
		const firstCard = page.getByRole("article", {
			name: /Feed: AI Trends/i,
		});
		await expect(firstCard).toBeVisible();

		// Verify show details button exists (rendered by FeedDetails component)
		const detailsButton = firstCard.getByRole("button", {
			name: /show details/i,
		});
		await expect(detailsButton).toBeVisible();
	});
});
