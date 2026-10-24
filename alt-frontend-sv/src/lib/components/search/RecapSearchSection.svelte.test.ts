import { page } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import type { RecapSectionData } from "#lib/connect/global_search.js";

const { gotoMock } = vi.hoisted(() => ({
	gotoMock: vi.fn(),
}));

vi.mock("$app/navigation", () => ({
	goto: gotoMock,
}));

import RecapSearchSection from "./RecapSearchSection.svelte";

const sectionFixture: RecapSectionData = {
	hits: [
		{
			id: "recap-1",
			jobId: "job-1",
			executedAt: "2026-04-07T00:00:00Z",
			windowDays: 7,
			genre: "AI & Machine Learning",
			summary: "Transformer inference efficiency and quantization advances.",
			topTerms: ["quantization", "inference", "gpu"],
			tags: ["ai", "ml"],
		},
	],
	estimatedTotal: 5,
	hasMore: true,
};

describe("RecapSearchSection", () => {
	beforeEach(() => {
		gotoMock.mockReset();
	});

	it("renders recap hits with genre, window badge, summary, and top terms", async () => {
		render(RecapSearchSection, {
			props: {
				section: sectionFixture,
				query: "quantization",
			},
		});

		await expect.element(page.getByText(/RECAPS/)).toBeInTheDocument();
		await expect.element(page.getByText("(5)")).toBeInTheDocument();
		await expect
			.element(page.getByText("AI & Machine Learning"))
			.toBeInTheDocument();
		await expect.element(page.getByText("7-day")).toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					"Transformer inference efficiency and quantization advances.",
				),
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText("quantization", { exact: true }))
			.toBeInTheDocument();
	});

	it("navigates to /recap?q=... when See all > is clicked", async () => {
		render(RecapSearchSection, {
			props: {
				section: sectionFixture,
				query: "distributed systems",
			},
		});

		const seeAllBtn = page.getByRole("button", { name: /see all/i });
		await expect.element(seeAllBtn).toBeInTheDocument();
		await seeAllBtn.click();

		expect(gotoMock).toHaveBeenCalledWith("/recap?q=distributed%20systems");
	});
});
