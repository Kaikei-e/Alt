import { describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { FeedInflowStatItem } from "#lib/connect/feeds.js";
import type { FeedLink } from "#lib/schema/feedLink.js";
import FeedInflowSection from "./FeedInflowSection.svelte";

const FEED_LINKS: FeedLink[] = [
	{
		id: "11111111-1111-4111-8111-111111111111",
		url: "https://www.high-volume.example/feed.xml?fmt=rss",
		healthStatus: "healthy",
		consecutiveFailures: 0,
		lastFailureReason: "",
		isActive: true,
	},
	{
		id: "22222222-2222-4222-8222-222222222222",
		url: "https://quiet.example/rss",
		healthStatus: "healthy",
		consecutiveFailures: 0,
		lastFailureReason: "",
		isActive: true,
	},
	{
		id: "33333333-3333-4333-8333-333333333333",
		url: "https://alpha-well-read.example/atom.xml",
		healthStatus: "healthy",
		consecutiveFailures: 0,
		lastFailureReason: "",
		isActive: true,
	},
];

function makeStats(): FeedInflowStatItem[] {
	return [
		{
			feedLinkId: "11111111-1111-4111-8111-111111111111",
			url: "https://high-volume.example/feed.xml",
			dailyCap: undefined,
			inflowCount: 1200,
			readCount: 60,
			openedCountLegacy: 14,
			readRate: 0.05,
		},
		{
			feedLinkId: "22222222-2222-4222-8222-222222222222",
			url: "https://quiet.example/rss",
			dailyCap: undefined,
			inflowCount: 0,
			readCount: 0,
			openedCountLegacy: 0,
			readRate: 0,
		},
		{
			feedLinkId: "33333333-3333-4333-8333-333333333333",
			url: "https://alpha-well-read.example/atom.xml",
			dailyCap: 10,
			inflowCount: 100,
			readCount: 40,
			openedCountLegacy: 0,
			readRate: 0.4,
		},
	];
}

describe("FeedInflowSection", () => {
	it("renders heading, help text, figures, en-dash for zero inflow, and legacy opened count only when > 0", async () => {
		render(FeedInflowSection, {
			stats: makeStats(),
			feedLinks: FEED_LINKS,
			isLoading: false,
			error: null,
			onRetry: vi.fn(),
			onSaveCap: vi.fn(),
		});

		await expect
			.element(page.getByRole("heading", { name: "Inflow, last 30 days" }))
			.toBeVisible();
		await expect
			.element(
				page.getByText(
					"A daily cap limits how many articles from a feed appear in Swipe and Gallery each day. Search still finds the rest.",
				),
			)
			.toBeVisible();

		const rows = page.getByTestId("inflow-row").elements();
		expect(rows).toHaveLength(3);

		// Default sort: Most inflow -> high-volume (1,200), alpha-well-read (100), quiet (0)
		// Hostname strips leading www. and path + query sits on the second line
		expect(rows[0]?.textContent).toContain("high-volume.example");
		expect(rows[0]?.textContent).not.toContain("www.high-volume.example");
		expect(rows[0]?.textContent).toContain("/feed.xml?fmt=rss");
		expect(
			rows[0]?.querySelector(
				"[title='https://www.high-volume.example/feed.xml?fmt=rss']",
			),
		).not.toBeNull();
		expect(rows[0]?.textContent).toContain("1,200");
		expect(rows[0]?.textContent).toContain("60");
		expect(rows[0]?.textContent).toContain("5%");
		expect(rows[0]?.textContent).toContain("Opened (older): 14");
		expect(rows[0]?.textContent).toContain("No cap");

		expect(rows[1]?.textContent).toContain("alpha-well-read.example");
		expect(rows[1]?.textContent).toContain("/atom.xml");
		expect(rows[1]?.textContent).toContain("40%");
		expect(rows[1]?.textContent).toContain("10 per day");
		expect(rows[1]?.textContent).not.toContain("Opened (older)");

		expect(rows[2]?.textContent).toContain("quiet.example");
		expect(rows[2]?.textContent).toContain("/rss");
		expect(rows[2]?.textContent).toContain("–");
		expect(rows[2]?.textContent).not.toContain("Opened (older)");
	});

	it("sorts client-side by Most inflow, Lowest read rate (zero inflow last), and Name", async () => {
		render(FeedInflowSection, {
			stats: makeStats(),
			feedLinks: FEED_LINKS,
			isLoading: false,
			error: null,
			onRetry: vi.fn(),
			onSaveCap: vi.fn(),
		});

		const select = page.getByTestId("inflow-sort-select");
		await userEvent.selectOptions(select, "lowest-read-rate");

		let rows = page.getByTestId("inflow-row").elements();
		expect(rows.map((r) => r.getAttribute("data-feed-link-id"))).toEqual([
			"11111111-1111-4111-8111-111111111111", // 5%
			"33333333-3333-4333-8333-333333333333", // 40%
			"22222222-2222-4222-8222-222222222222", // 0 inflow -> last
		]);

		await userEvent.selectOptions(select, "name");
		rows = page.getByTestId("inflow-row").elements();
		expect(rows.map((r) => r.getAttribute("data-feed-link-id"))).toEqual([
			"33333333-3333-4333-8333-333333333333", // alpha
			"11111111-1111-4111-8111-111111111111", // high-volume
			"22222222-2222-4222-8222-222222222222", // quiet
		]);
	});

	it("optimistically updates daily cap and rolls back with role=alert on failure", async () => {
		let rejectSave!: (err: Error) => void;
		const onSaveCap = vi.fn(
			() =>
				new Promise<number | undefined>((_resolve, reject) => {
					rejectSave = reject;
				}),
		);

		render(FeedInflowSection, {
			stats: makeStats(),
			feedLinks: FEED_LINKS,
			isLoading: false,
			error: null,
			onRetry: vi.fn(),
			onSaveCap,
		});

		const firstRow = page.getByTestId("inflow-row").first();
		const editBtn = firstRow.getByTestId("inflow-cap-edit");
		expect(
			editBtn.element().getBoundingClientRect().height,
		).toBeGreaterThanOrEqual(44);
		await editBtn.click();

		const input = firstRow.getByTestId("inflow-cap-input");
		await expect.element(input).toHaveAttribute("inputmode", "numeric");
		await expect.element(input).toHaveAttribute("min", "1");
		await expect.element(input).toHaveAttribute("max", "500");
		const inputRect = input.element().getBoundingClientRect();
		expect(inputRect.width).toBeLessThanOrEqual(110);
		expect(inputRect.height).toBeGreaterThanOrEqual(44);
		await input.fill("25");

		await firstRow.getByTestId("inflow-cap-save").click();

		// Optimistic update shows 25 per day immediately
		await expect
			.element(firstRow.getByTestId("inflow-cap-value"))
			.toHaveTextContent("25 per day");

		// Reject -> rolls back to No cap and shows inline role=alert error
		rejectSave(new Error("Could not save daily cap."));
		await expect
			.element(firstRow.getByTestId("inflow-cap-value"))
			.toHaveTextContent("No cap");
		await expect.element(firstRow.getByRole("alert")).toBeVisible();
	});

	it("validates cap range (1..500 integer) before calling onSaveCap", async () => {
		const onSaveCap = vi.fn();

		render(FeedInflowSection, {
			stats: makeStats(),
			feedLinks: FEED_LINKS,
			isLoading: false,
			error: null,
			onRetry: vi.fn(),
			onSaveCap,
		});

		const firstRow = page.getByTestId("inflow-row").first();
		await firstRow.getByTestId("inflow-cap-edit").click();
		await firstRow.getByTestId("inflow-cap-input").fill("0");
		await firstRow.getByTestId("inflow-cap-save").click();

		expect(onSaveCap).not.toHaveBeenCalled();
		await expect.element(firstRow.getByRole("alert")).toBeVisible();
	});

	it("renders loading, error with Retry, and empty states", async () => {
		const onRetry = vi.fn();
		const { rerender } = render(FeedInflowSection, {
			stats: [],
			feedLinks: [],
			isLoading: true,
			error: null,
			onRetry,
			onSaveCap: vi.fn(),
		});

		await expect.element(page.getByTestId("inflow-loading")).toBeVisible();

		await rerender({
			stats: [],
			feedLinks: [],
			isLoading: false,
			error: "Network error",
			onRetry,
			onSaveCap: vi.fn(),
		});
		await expect.element(page.getByTestId("inflow-error")).toBeVisible();
		const retryBtn = page.getByRole("button", { name: "Retry" });
		expect(
			retryBtn.element().getBoundingClientRect().height,
		).toBeGreaterThanOrEqual(44);
		await retryBtn.click();
		expect(onRetry).toHaveBeenCalledTimes(1);

		await rerender({
			stats: [],
			feedLinks: [],
			isLoading: false,
			error: null,
			onRetry,
			onSaveCap: vi.fn(),
		});
		await expect.element(page.getByText("No subscribed feeds.")).toBeVisible();
	});
});
