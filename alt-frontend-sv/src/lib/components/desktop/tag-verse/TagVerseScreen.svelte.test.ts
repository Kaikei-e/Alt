import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagCloudData } from "#lib/connect/articles.js";

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
			.mockResolvedValueOnce({
				tags: [
					{
						tagName: "svelte",
						articleCount: 1,
						positionX: 0,
						positionY: 0,
						positionZ: 0,
						clusterId: 0,
					},
				],
				edges: [],
			});

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
		let resolveCloud: ((data: TagCloudData) => void) | undefined;
		mockFetchTagCloud.mockReturnValueOnce(
			new Promise<TagCloudData>((resolve) => {
				resolveCloud = resolve;
			}),
		);

		const view = render(TagVerseScreen);

		const statusRegion = page.getByRole("status");
		await expect
			.element(statusRegion, { timeout: 1000 })
			.toHaveTextContent("Loading tags");

		resolveCloud?.({ tags: [], edges: [] });
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
		mockFetchTagCloud.mockResolvedValue({
			tags: [
				{
					tagName: "svelte",
					articleCount: 10,
					positionX: 0,
					positionY: 0,
					positionZ: 0,
					clusterId: 0,
				},
				{
					tagName: "rust",
					articleCount: 20,
					positionX: 1,
					positionY: 1,
					positionZ: 1,
					clusterId: 0,
				},
			],
			edges: [],
		});

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

	it("overlays the detail panel without changing the canvas container width", async () => {
		mockFetchTagCloud.mockResolvedValue({
			tags: [
				{
					tagName: "rust",
					articleCount: 50,
					positionX: 1,
					positionY: 0,
					positionZ: 0,
					clusterId: 1,
				},
			],
			edges: [],
		});

		const { container } = render(TagVerseScreen);

		const rustBtn = page.getByRole("button", { name: "rust, 50 articles" });
		await expect.element(rustBtn, { timeout: 1000 }).toBeInTheDocument();

		const canvasRegion = container.querySelector<HTMLElement>(
			'[data-testid="tag-verse-canvas-region"]',
		);
		expect(canvasRegion).not.toBeNull();
		const widthBeforeOpen = canvasRegion?.getBoundingClientRect().width ?? 0;
		expect(widthBeforeOpen).toBeGreaterThan(0);

		await rustBtn.click();
		const rustHeading = page.getByRole("heading", {
			name: "rust",
			exact: true,
			level: 2,
		});
		await expect.element(rustHeading).toBeInTheDocument();

		const widthAfterOpen = canvasRegion?.getBoundingClientRect().width ?? 0;
		expect(widthAfterOpen).toBe(widthBeforeOpen);
	});

	it("renders Topics key buttons that emphasise clusters on hover/focus and hides the key while a tag is selected", async () => {
		mockFetchTagCloud.mockResolvedValue({
			tags: [
				{
					tagName: "rust",
					articleCount: 50,
					positionX: 1,
					positionY: 0,
					positionZ: 0,
					clusterId: 1,
				},
				{
					tagName: "tokio",
					articleCount: 30,
					positionX: 2,
					positionY: 0,
					positionZ: 0,
					clusterId: 1,
				},
				{
					tagName: "svelte",
					articleCount: 40,
					positionX: -1,
					positionY: 0,
					positionZ: 0,
					clusterId: 2,
				},
				{
					tagName: "vite",
					articleCount: 20,
					positionX: -2,
					positionY: 0,
					positionZ: 0,
					clusterId: 2,
				},
			],
			edges: [{ sourceIndex: 0, targetIndex: 1, weight: 0.9 }],
		});

		render(TagVerseScreen);

		const topicsRegion = page.getByRole("region", { name: /topics/i });
		await expect.element(topicsRegion, { timeout: 1000 }).toBeInTheDocument();
		const cluster1Btn = page.getByRole("button", {
			name: /rust\s*·\s*tokio/i,
		});
		const cluster2Btn = page.getByRole("button", {
			name: /svelte\s*·\s*vite/i,
		});
		await expect.element(cluster1Btn).toBeInTheDocument();

		await cluster2Btn.hover();
		await expect
			.element(cluster2Btn)
			.toHaveAttribute("data-emphasized", "true");

		const cluster1El = (await cluster1Btn.element()) as HTMLButtonElement;
		cluster1El.focus();
		await expect
			.element(cluster1Btn)
			.toHaveAttribute("data-emphasized", "true");
		await expect
			.element(cluster2Btn)
			.toHaveAttribute("data-emphasized", "false");

		const rustIndexBtn = page.getByRole("button", {
			name: "rust, 50 articles",
		});
		await rustIndexBtn.click();
		await expect.element(topicsRegion).not.toBeInTheDocument();
	});
});
