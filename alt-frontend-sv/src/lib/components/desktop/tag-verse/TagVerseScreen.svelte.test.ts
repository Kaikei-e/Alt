import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

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
	detectGPUBackend: vi.fn(() => "webgl2"),
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

	it("renders Alt-Paper top bar with tag and article totals and retries load without page reload", async () => {
		mockFetchTagCloud
			.mockRejectedValueOnce(new Error("Network failure"))
			.mockResolvedValueOnce([
				{
					tagName: "svelte",
					articleCount: 10,
					positionX: 0,
					positionY: 0,
					positionZ: 0,
				},
				{
					tagName: "rust",
					articleCount: 15,
					positionX: 1,
					positionY: 1,
					positionZ: 1,
				},
			]);

		render(TagVerseScreen);

		await expect
			.element(page.getByRole("heading", { name: "Tag Verse", level: 1 }))
			.toBeInTheDocument();

		const retryButton = page.getByRole("button", { name: "Retry" });
		await expect.element(retryButton).toBeInTheDocument();
		await retryButton.click();

		await expect
			.element(page.getByText("2 tags · 25 articles"))
			.toBeInTheDocument();
		expect(mockFetchTagCloud).toHaveBeenCalledTimes(2);
	});
});
