import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagCloudItem } from "#lib/connect/index.js";

const { mockFetchTagCloud, mockIsWebGL2Available } = vi.hoisted(() => ({
	mockFetchTagCloud: vi.fn(),
	mockIsWebGL2Available: vi.fn(() => true),
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: vi.fn(() => ({})),
	fetchTagCloud: mockFetchTagCloud,
	fetchArticlesByTag: vi.fn().mockResolvedValue({
		articles: [],
		nextCursor: null,
		hasMore: false,
	}),
	searchRecapsByTag: vi.fn().mockResolvedValue([]),
}));

vi.mock("#lib/utils/gpuCapability.js", () => ({
	isWebGL2Available: mockIsWebGL2Available,
}));

vi.mock("./TagVerseScene.svelte", () => ({
	default: () => {},
}));

import TagVerseScreen from "./TagVerseScreen.svelte";

describe("TagVerseScreen", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockIsWebGL2Available.mockReturnValue(true);
	});

	it("renders Alt-Paper top bar, role='alert' on error, and retries load without page reload", async () => {
		mockFetchTagCloud
			.mockRejectedValueOnce(new Error("Network failure"))
			.mockResolvedValueOnce([
				{
					tagName: "svelte",
					articleCount: 1,
					positionX: 0,
					positionY: 0,
					positionZ: 0,
				},
			]);

		render(TagVerseScreen);

		await expect
			.element(page.getByRole("heading", { name: "Tag Verse", level: 1 }))
			.toBeInTheDocument();

		const alertRegion = page.getByRole("alert");
		await expect
			.element(alertRegion, { timeout: 1000 })
			.toHaveTextContent("Network failure");

		const retryButton = page.getByRole("button", { name: "Retry" });
		await retryButton.click();

		await expect
			.element(page.getByText("1 tag · 1 article"), { timeout: 1000 })
			.toBeInTheDocument();
		expect(mockFetchTagCloud).toHaveBeenCalledTimes(2);
	});

	it("renders loading with role='status', empty state text, and WebGL-unavailable state text", async () => {
		let resolveTags: ((tags: TagCloudItem[]) => void) | undefined;
		mockFetchTagCloud.mockReturnValueOnce(
			new Promise<TagCloudItem[]>((resolve) => {
				resolveTags = resolve;
			}),
		);

		const view = render(TagVerseScreen);

		const statusRegion = page.getByRole("status");
		await expect
			.element(statusRegion, { timeout: 1000 })
			.toHaveTextContent("Loading tags");

		resolveTags?.([]);
		await expect
			.element(
				page.getByText(
					"No tags yet. Tags appear here once articles are tagged.",
				),
				{ timeout: 1000 },
			)
			.toBeInTheDocument();
		view.unmount();

		mockIsWebGL2Available.mockReturnValue(false);
		render(TagVerseScreen);
		await expect
			.element(
				page.getByText(
					"Tag Verse needs WebGL 2, which this browser does not provide.",
				),
				{ timeout: 1000 },
			)
			.toBeInTheDocument();
	});

	it("keeps detail panel open when Escape is pressed in 'Find a tag' search box and moves focus to new heading when switching tags", async () => {
		mockFetchTagCloud.mockResolvedValue([
			{
				tagName: "svelte",
				articleCount: 10,
				positionX: 0,
				positionY: 0,
				positionZ: 0,
			},
			{
				tagName: "rust",
				articleCount: 20,
				positionX: 1,
				positionY: 1,
				positionZ: 1,
			},
		]);

		render(TagVerseScreen);

		const svelteBtn = page.getByRole("button", {
			name: "svelte, 10 articles",
		});
		await expect.element(svelteBtn, { timeout: 1000 }).toBeInTheDocument();
		await svelteBtn.click();

		const svelteHeading = page.getByRole("heading", {
			name: "svelte",
			exact: true,
			level: 2,
		});
		await expect.element(svelteHeading).toBeInTheDocument();

		const searchInput = page.getByLabelText("Find a tag");
		const searchInputEl = await searchInput.element();
		(searchInputEl as HTMLElement).focus();
		searchInputEl.dispatchEvent(
			new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
		);

		await expect.element(svelteHeading).toBeInTheDocument();

		const rustBtn = page.getByRole("button", { name: "rust, 20 articles" });
		await rustBtn.click();

		const rustHeading = page.getByRole("heading", {
			name: "rust",
			exact: true,
			level: 2,
		});
		await expect.element(rustHeading).toBeInTheDocument();
		const rustHeadingEl = await rustHeading.element();
		expect(document.activeElement).toBe(rustHeadingEl);
	});
});
