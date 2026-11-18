import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

vi.mock("#lib/api/client/index.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		deleteFeedLinkClient: vi.fn(),
		getFeedInflowStatsClient: vi.fn(),
		listFeedLinksClient: vi.fn(),
		listSubscriptionsClient: vi.fn(),
		registerRssFeedClient: vi.fn(),
		setSubscriptionDailyCapClient: vi.fn(),
		subscribeClient: vi.fn(),
		unsubscribeClient: vi.fn(),
	};
});

import {
	getFeedInflowStatsClient,
	listSubscriptionsClient,
	setSubscriptionDailyCapClient,
} from "#lib/api/client/index.js";
import Page from "./+page.svelte";

const FEED_LINKS = [
	{
		id: "11111111-1111-4111-8111-111111111111",
		url: "https://example.com/feed.xml",
		healthStatus: "healthy" as const,
		consecutiveFailures: 0,
		lastFailureReason: "",
		isActive: true,
	},
];

describe("/settings/feeds page inflow integration", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(listSubscriptionsClient).mockResolvedValue([]);
	});

	it("loads inflow stats on mount and supports setting and clearing daily cap", async () => {
		vi.mocked(getFeedInflowStatsClient).mockResolvedValue({
			windowDays: 30,
			stats: [
				{
					feedLinkId: "11111111-1111-4111-8111-111111111111",
					url: "https://example.com/feed.xml",
					dailyCap: undefined,
					inflowCount: 250,
					readCount: 25,
					openedCountLegacy: 0,
					readRate: 0.1,
				},
			],
		});
		vi.mocked(setSubscriptionDailyCapClient)
			.mockResolvedValueOnce({ message: "Daily cap updated", dailyCap: 12 })
			.mockResolvedValueOnce({
				message: "Daily cap cleared",
				dailyCap: undefined,
			});

		render(Page, {
			data: {
				feedLinks: FEED_LINKS,
			},
		});

		await expect
			.element(page.getByRole("heading", { name: "Inflow, last 30 days" }))
			.toBeVisible();
		const row = page.getByTestId("inflow-row").first();
		await expect.element(row).toBeVisible();
		await expect
			.element(row.getByTestId("inflow-cap-value"))
			.toHaveTextContent("No cap");

		await row.getByTestId("inflow-cap-edit").click();
		await row.getByTestId("inflow-cap-input").fill("12");
		await row.getByTestId("inflow-cap-save").click();

		expect(setSubscriptionDailyCapClient).toHaveBeenCalledWith(
			"11111111-1111-4111-8111-111111111111",
			12,
		);
		await expect
			.element(row.getByTestId("inflow-cap-value"))
			.toHaveTextContent("12 per day");

		await row.getByTestId("inflow-cap-edit").click();
		await row.getByTestId("inflow-cap-clear").click();

		expect(setSubscriptionDailyCapClient).toHaveBeenCalledWith(
			"11111111-1111-4111-8111-111111111111",
			undefined,
		);
		await expect
			.element(row.getByTestId("inflow-cap-value"))
			.toHaveTextContent("No cap");
	});
});
