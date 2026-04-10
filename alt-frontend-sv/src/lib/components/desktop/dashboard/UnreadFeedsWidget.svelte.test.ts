import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import UnreadFeedsWidget from "./UnreadFeedsWidget.svelte";
import { MOCK_FEEDS } from "./dashboard-fixtures";

describe("UnreadFeedsWidget", () => {
	it("renders LATEST DISPATCHES heading", async () => {
		render(UnreadFeedsWidget as never, {
			props: { feeds: MOCK_FEEDS, isLoading: false, error: null },
		});

		await expect
			.element(page.getByText("LATEST DISPATCHES"))
			.toBeInTheDocument();
	});

	it("renders feed titles when loaded", async () => {
		render(UnreadFeedsWidget as never, {
			props: { feeds: MOCK_FEEDS, isLoading: false, error: null },
		});

		await expect
			.element(
				page.getByText("TSMC Expands 3nm Capacity to Meet AI Chip Demand"),
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText("OpenAI Releases GPT-5 with Native Tool Use"))
			.toBeInTheDocument();
	});

	it("renders feed excerpts", async () => {
		render(UnreadFeedsWidget as never, {
			props: { feeds: MOCK_FEEDS, isLoading: false, error: null },
		});

		await expect
			.element(
				page.getByText(/Taiwan Semiconductor Manufacturing Co\. announced/),
			)
			.toBeInTheDocument();
	});

	it("shows loading state with pulsing dot text", async () => {
		render(UnreadFeedsWidget as never, {
			props: { feeds: [], isLoading: true, error: null },
		});

		await expect
			.element(page.getByText(/retrieving dispatches/i))
			.toBeInTheDocument();
	});

	it("shows empty state", async () => {
		render(UnreadFeedsWidget as never, {
			props: { feeds: [], isLoading: false, error: null },
		});

		await expect.element(page.getByText(/no dispatches/i)).toBeInTheDocument();
	});

	it("shows error state", async () => {
		render(UnreadFeedsWidget as never, {
			props: {
				feeds: [],
				isLoading: false,
				error: new Error("Network failure"),
			},
		});

		await expect.element(page.getByText("Network failure")).toBeInTheDocument();
	});

	it("renders View All link", async () => {
		render(UnreadFeedsWidget as never, {
			props: { feeds: MOCK_FEEDS, isLoading: false, error: null },
		});

		const link = page.getByRole("link", { name: /view all/i });
		await expect.element(link).toBeInTheDocument();
		await expect.element(link).toHaveAttribute("href", "/feeds");
	});
});
