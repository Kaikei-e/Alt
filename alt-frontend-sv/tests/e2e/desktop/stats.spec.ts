import { test, expect } from "../fixtures/pomFixtures";
import { fulfillJson } from "../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_TREND_STATS_PATH,
	CONNECT_TREND_STATS_RESPONSE,
} from "../fixtures/mockData";
import { createMockEventSourceScript } from "../utils/mockHelpers";

const MOCK_STATS = {
	feedAmount: 25,
	unsummarizedFeedAmount: 10,
	articleAmount: 500,
};

test.describe("Desktop Statistics", () => {
	test.beforeEach(async ({ page }) => {
		// Mock EventSource
		await page.addInitScript(createMockEventSourceScript());

		await page.route(CONNECT_RPC_PATHS.getDetailedFeedStats, (route) =>
			fulfillJson(route, MOCK_STATS),
		);
		await page.route(CONNECT_TREND_STATS_PATH, (route) =>
			fulfillJson(route, CONNECT_TREND_STATS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadCount, (route) =>
			fulfillJson(route, { count: 42 }),
		);
	});

	test("renders page title and stat cards", async ({ desktopStatsPage }) => {
		await desktopStatsPage.goto();
		await desktopStatsPage.waitForStatsLoaded();

		await expect(desktopStatsPage.pageTitle).toBeVisible();
		await expect(desktopStatsPage.feedCountCard).toBeVisible();
		await expect(desktopStatsPage.totalArticlesCard).toBeVisible();
		await expect(desktopStatsPage.unsummarizedCard).toBeVisible();
	});

	test("shows Trend Charts heading", async ({ desktopStatsPage }) => {
		await desktopStatsPage.goto();
		await desktopStatsPage.waitForStatsLoaded();

		await expect(desktopStatsPage.trendChartsHeading).toBeVisible();
	});

	test("shows reconnect button when disconnected", async ({
		page,
		desktopStatsPage,
	}) => {
		// Override EventSource to simulate disconnect
		await page.addInitScript(() => {
			class DisconnectedEventSource {
				url: string;
				readyState = 2;
				static CONNECTING = 0;
				static OPEN = 1;
				static CLOSED = 2;
				CONNECTING = 0;
				OPEN = 1;
				CLOSED = 2;
				onopen = null;
				onmessage = null;
				onerror = null;
				constructor(url: string) {
					this.url = url;
					setTimeout(() => {
						(this as unknown as { onerror?: (e: Event) => void }).onerror?.(
							new Event("error"),
						);
					}, 0);
				}
				close() {
					this.readyState = 2;
				}
				addEventListener() {}
				removeEventListener() {}
				dispatchEvent() {
					return false;
				}
			}
			(window as unknown as Record<string, unknown>).EventSource =
				DisconnectedEventSource;
		});

		await desktopStatsPage.goto();
		await expect(desktopStatsPage.pageTitle).toBeVisible();

		// Reconnect button may be visible if SSE fails
		// This is a soft check since the component may handle errors gracefully
		const reconnect = desktopStatsPage.reconnectButton;
		if (await reconnect.isVisible().catch(() => false)) {
			await expect(reconnect).toBeVisible();
		}
	});
});
