import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	getDetailedFeedStatsClient,
	getUnreadCountClient,
} from "$lib/api/client/feeds";
import { getTrendStats } from "$lib/api/client/stats";
import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";

// Spy-mode automocks keep every export bound and let each test override only
// the network calls.
vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$lib/api/client/stats", { spy: true });
vi.mock("$lib/hooks/useFeedStats.svelte", { spy: true });

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 80));
}

describe("Stats page", () => {
	beforeEach(() => {
		vi.mocked(getTrendStats).mockReset();
		vi.mocked(getTrendStats).mockResolvedValue({
			data_points: [],
			granularity: "hourly",
			window: "24h",
		});
		vi.mocked(getDetailedFeedStatsClient).mockReset();
		vi.mocked(getDetailedFeedStatsClient).mockResolvedValue({
			feed_amount: { amount: 12 },
			total_articles: { amount: 345 },
			unsummarized_articles: { amount: 6 },
		});
		vi.mocked(getUnreadCountClient).mockReset();
		vi.mocked(getUnreadCountClient).mockResolvedValue({ count: 7 });
		vi.mocked(useFeedStats).mockReturnValue({
			feedAmount: 0,
			unsummarizedArticlesAmount: 0,
			totalArticlesAmount: 0,
			isConnected: false,
			retryCount: 0,
			reconnect: () => {},
		});
	});

	it("fetches only the figures the opening viewport actually shows", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		expect(getTrendStats).toHaveBeenCalledTimes(1);
		expect(getDetailedFeedStatsClient).not.toHaveBeenCalled();
	});

	it("fills in the phone ledger when the phone is rotated upright", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// The two viewports show different figures from different endpoints, and
		// `onMount` fetched exactly one set — so rotating used to land on a
		// ledger that stayed on "Loading…" for good.
		await expect
			.element(page.getByTestId("stat-total-articles"))
			.toHaveTextContent("345");
		await expect
			.element(page.getByTestId("stat-unread-count"))
			.toHaveTextContent("7");
	});

	it("fills in the trend charts when a phone ledger is rotated into landscape", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();
		expect(getTrendStats).not.toHaveBeenCalled();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		expect(getTrendStats).toHaveBeenCalledTimes(1);
	});

	it("does not re-fetch a side it has already loaded", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		// Rotation is cheap and repeatable; a reader flipping the phone must not
		// spend a request each way.
		expect(getTrendStats).toHaveBeenCalledTimes(1);
		expect(getDetailedFeedStatsClient).toHaveBeenCalledTimes(1);
	});
});
