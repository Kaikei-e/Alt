import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagTrailArticlesResponse } from "#lib/connect/index.js";

const { mockFetchArticlesByTag, mockSearchRecapsByTag } = vi.hoisted(() => ({
	mockFetchArticlesByTag: vi.fn(),
	mockSearchRecapsByTag: vi.fn(),
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: vi.fn(() => ({})),
	fetchArticlesByTag: mockFetchArticlesByTag,
	searchRecapsByTag: mockSearchRecapsByTag,
}));

import HUDArticlesTab from "./HUDArticlesTab.svelte";
import TagVerseHUD from "./TagVerseHUD.svelte";

describe("TagVerseHUD and HUDArticlesTab", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockFetchArticlesByTag.mockResolvedValue({
			articles: [
				{
					id: "art-1",
					title: "Understanding Svelte 5 Runes",
					link: "https://example.com/svelte-5",
					publishedAt: "2026-08-01T00:00:00Z",
					feedTitle: "Svelte Blog",
				},
			],
			nextCursor: null,
			hasMore: false,
		});
		mockSearchRecapsByTag.mockResolvedValue([]);
	});

	it("exposes tablist/tab with aria-selected and switches tabs with Left/Right arrow keys without unmounting inactive tab", async () => {
		render(TagVerseHUD, {
			props: {
				tagName: "svelte",
				articleCount: 1,
				onClose: vi.fn(),
			},
		});

		const tablist = page.getByRole("tablist");
		await expect.element(tablist).toBeInTheDocument();
		await expect
			.element(page.getByText("1 article", { exact: true }), { timeout: 1000 })
			.toBeInTheDocument();

		const articlesTab = page.getByRole("tab", { name: "Articles" });
		const recapsTab = page.getByRole("tab", { name: "Recaps" });

		await expect.element(articlesTab).toHaveAttribute("aria-selected", "true");
		await expect.element(recapsTab).toHaveAttribute("aria-selected", "false");

		const articlesEl = await articlesTab.element();
		(articlesEl as HTMLElement).focus();
		articlesEl.dispatchEvent(
			new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
		);

		await expect.element(recapsTab).toHaveAttribute("aria-selected", "true");
		await expect.element(articlesTab).toHaveAttribute("aria-selected", "false");

		const recapsEl = await recapsTab.element();
		recapsEl.dispatchEvent(
			new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }),
		);

		await expect.element(articlesTab).toHaveAttribute("aria-selected", "true");
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(1);
	});

	it("calls onClose on Escape, ignores Escape during IME composition or from external inputs, and moves focus to new heading on tag change", async () => {
		const opener = document.createElement("button");
		opener.textContent = "Index Tag Opener";
		document.body.appendChild(opener);

		const externalInput = document.createElement("input");
		externalInput.type = "search";
		document.body.appendChild(externalInput);

		opener.focus();

		const onClose = vi.fn();
		const view = render(TagVerseHUD, {
			props: {
				tagName: "svelte",
				articleCount: 12,
				onClose,
			},
		});

		try {
			const heading = page.getByRole("heading", {
				name: "svelte",
				exact: true,
				level: 2,
			});
			await expect.element(heading).toBeInTheDocument();
			const headingEl = await heading.element();
			expect(document.activeElement).toBe(headingEl);

			// Escape during IME composition must not close
			window.dispatchEvent(
				new KeyboardEvent("keydown", {
					key: "Escape",
					isComposing: true,
					bubbles: true,
				}),
			);
			expect(onClose).not.toHaveBeenCalled();

			// Escape from external search input must not close
			externalInput.dispatchEvent(
				new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
			);
			expect(onClose).not.toHaveBeenCalled();

			// Changing tagName while open focuses the updated heading
			externalInput.focus();
			await view.rerender({
				tagName: "rust",
				articleCount: 45,
				onClose,
			});
			const rustHeading = page.getByRole("heading", {
				name: "rust",
				exact: true,
				level: 2,
			});
			await expect.element(rustHeading).toBeInTheDocument();
			const rustHeadingEl = await rustHeading.element();
			expect(document.activeElement).toBe(rustHeadingEl);

			// Plain Escape closes
			window.dispatchEvent(
				new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
			);

			expect(onClose).toHaveBeenCalledTimes(1);
			view.unmount();
		} finally {
			opener.remove();
			externalInput.remove();
		}
	});

	it("HUDArticlesTab calls fetchArticlesByTag once on mount, only when tagName changes, and ignores late load-more from previous tag after A -> B -> A", async () => {
		let resolveLoadMoreA: ((val: TagTrailArticlesResponse) => void) | undefined;

		mockFetchArticlesByTag.mockImplementation(
			(_transport: unknown, tag: string, _tagId?: string, cursor?: string) => {
				if (tag === "svelte" && cursor === "cursor-a-2") {
					return new Promise<TagTrailArticlesResponse>((resolve) => {
						resolveLoadMoreA = resolve;
					});
				}
				return Promise.resolve({
					articles: [
						{
							id: `art-${tag}-1`,
							title: `Initial article for ${tag}`,
							link: `https://example.com/${tag}`,
							publishedAt: "2026-08-01T00:00:00Z",
						},
					],
					nextCursor: tag === "svelte" ? "cursor-a-2" : null,
					hasMore: tag === "svelte",
				});
			},
		);

		const view = render(HUDArticlesTab, {
			props: {
				tagName: "svelte",
			},
		});

		await expect
			.element(page.getByText("Initial article for svelte"))
			.toBeInTheDocument();
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(1);

		await view.rerender({ tagName: "svelte" });
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(1);

		// Start load more on tag A ("svelte")
		const loadMoreBtn = page.getByRole("button", { name: "Load more" });
		await loadMoreBtn.click();
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(2);

		// Switch A -> B ("rust") -> A ("svelte") before A's load-more resolves
		await view.rerender({ tagName: "rust" });
		await expect
			.element(page.getByText("Initial article for rust"))
			.toBeInTheDocument();

		await view.rerender({ tagName: "svelte" });
		await expect
			.element(page.getByText("Initial article for svelte"))
			.toBeInTheDocument();

		// Resolve stale load-more from first "svelte" session
		resolveLoadMoreA?.({
			articles: [
				{
					id: "stale-page-2",
					title: "Stale Page 2 Article",
					link: "https://example.com/stale",
					publishedAt: "2026-08-01T00:00:00Z",
				},
			],
			nextCursor: null,
			hasMore: false,
		});

		await new Promise((r) => setTimeout(r, 30));
		expect(view.container.textContent).not.toContain("Stale Page 2 Article");
	});
});
