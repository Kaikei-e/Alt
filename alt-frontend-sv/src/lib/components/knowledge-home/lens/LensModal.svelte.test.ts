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

const tags = [
	{ name: "AI", count: 42 },
	{ name: "Rust", count: 15 },
	{ name: "Agents", count: 8 },
];

function renderModal(overrides: Record<string, unknown> = {}) {
	return render(LensModal as never, {
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
			availableTags: tags,
			onOpenChange: vi.fn(),
			onSave: vi.fn(),
			...overrides,
		},
	});
}

describe("LensModal", () => {
	it("renders sources instead of feed UUID input", async () => {
		renderModal();

		await expect
			.element(page.getByText("Save current view"))
			.toBeInTheDocument();
		await expect.element(page.getByText("Example Feed")).toBeInTheDocument();
		await expect.element(page.getByText("Feed IDs")).not.toBeInTheDocument();
	});

	it("displays required asterisk on Name field", async () => {
		renderModal();
		await expect.element(page.getByText("*")).toBeInTheDocument();
	});

	it("displays (optional) labels on non-required fields", async () => {
		renderModal();
		const optionalLabels = page.getByText("(optional)");
		// Description, Search query, Tags, Sources, Recent window = 5 optional fields
		await expect.element(optionalLabels.first()).toBeInTheDocument();
	});

	it("renders filter criteria section separator", async () => {
		renderModal();
		await expect.element(page.getByText(/Filter criteria/)).toBeInTheDocument();
	});

	it("renders TagCombobox for tag selection", async () => {
		renderModal();
		await expect
			.element(page.getByPlaceholder("Search or add tags..."))
			.toBeInTheDocument();
	});
});
