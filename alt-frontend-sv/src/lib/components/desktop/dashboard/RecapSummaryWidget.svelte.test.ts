import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import RecapSummaryWidget from "./RecapSummaryWidget.svelte";
import { MOCK_RECAP } from "./dashboard-fixtures";

describe("RecapSummaryWidget", () => {
	it("renders THREE-DAY BRIEF heading", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: MOCK_RECAP, isLoading: false, error: null },
		});

		await expect.element(page.getByText("THREE-DAY BRIEF")).toBeInTheDocument();
	});

	it("renders genre names", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: MOCK_RECAP, isLoading: false, error: null },
		});

		await expect.element(page.getByText("Technology")).toBeInTheDocument();
		await expect
			.element(page.getByText("Policy & Regulation"))
			.toBeInTheDocument();
		await expect.element(page.getByText("Research")).toBeInTheDocument();
	});

	it("renders genre summaries", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: MOCK_RECAP, isLoading: false, error: null },
		});

		await expect
			.element(page.getByText(/AI infrastructure spending continues/))
			.toBeInTheDocument();
	});

	it("renders article counts", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: MOCK_RECAP, isLoading: false, error: null },
		});

		await expect.element(page.getByText(/45 articles/)).toBeInTheDocument();
	});

	it("renders top terms inline", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: MOCK_RECAP, isLoading: false, error: null },
		});

		await expect.element(page.getByText(/GPU/)).toBeInTheDocument();
	});

	it("shows loading state", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: null, isLoading: true, error: null },
		});

		await expect
			.element(page.getByText(/retrieving brief/i))
			.toBeInTheDocument();
	});

	it("shows empty state when no data", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: null, isLoading: false, error: null },
		});

		await expect
			.element(page.getByText(/no briefing available/i))
			.toBeInTheDocument();
	});

	it("shows error state", async () => {
		render(RecapSummaryWidget as never, {
			props: {
				recapData: null,
				isLoading: false,
				error: new Error("Service unavailable"),
			},
		});

		await expect
			.element(page.getByText("Service unavailable"))
			.toBeInTheDocument();
	});

	it("renders updated timestamp", async () => {
		render(RecapSummaryWidget as never, {
			props: { recapData: MOCK_RECAP, isLoading: false, error: null },
		});

		await expect.element(page.getByText(/updated/i)).toBeInTheDocument();
	});
});
