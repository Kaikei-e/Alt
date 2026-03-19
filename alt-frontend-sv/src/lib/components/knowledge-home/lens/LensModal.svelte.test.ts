import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { ConnectFeedSource } from "$lib/connect";
import LensModal from "./LensModal.svelte";

const sources: ConnectFeedSource[] = [
	{
		id: "source-1",
		url: "https://example.com/rss",
		title: "Example Feed",
		isSubscribed: true,
		createdAt: "2026-03-20T00:00:00Z",
	},
];

describe("LensModal", () => {
	it("renders subscribed sources instead of feed UUID input", async () => {
		render(LensModal as never, {
			props: {
				open: true,
				version: {
					queryText: "agents",
					tagIds: ["AI"],
					sourceIds: ["source-1"],
					timeWindow: "7d",
					includeRecap: false,
					includePulse: false,
					sortMode: "relevance",
				},
				availableSources: sources,
				onOpenChange: vi.fn(),
				onSave: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Save current view"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Sources", { exact: true }))
			.toBeInTheDocument();
		await expect.element(page.getByText("Example Feed")).toBeInTheDocument();
		await expect.element(page.getByText("Feed IDs")).not.toBeInTheDocument();
	});
});
