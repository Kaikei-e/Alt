import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

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
				articleCount: 12,
				onClose: vi.fn(),
			},
		});

		const tablist = page.getByRole("tablist");
		await expect.element(tablist).toBeInTheDocument();

		const articlesTab = page.getByRole("tab", { name: "Articles" });
		const recapsTab = page.getByRole("tab", { name: "Recaps" });

		await expect
			.element(articlesTab)
			.toHaveAttribute("aria-selected", "true");
		await expect
			.element(recapsTab)
			.toHaveAttribute("aria-selected", "false");

		const articlesEl = await articlesTab.element();
		(articlesEl as HTMLElement).focus();
		articlesEl.dispatchEvent(
			new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
		);

		await expect
			.element(recapsTab)
			.toHaveAttribute("aria-selected", "true");
		await expect
			.element(articlesTab)
			.toHaveAttribute("aria-selected", "false");

		const recapsEl = await recapsTab.element();
		recapsEl.dispatchEvent(
			new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }),
		);

		await expect
			.element(articlesTab)
			.toHaveAttribute("aria-selected", "true");
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(1);
	});

	it("calls onClose when Escape is pressed on the detail panel and returns focus to the opener", async () => {
		const opener = document.createElement("button");
		opener.textContent = "Index Tag Opener";
		document.body.appendChild(opener);
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

			window.dispatchEvent(
				new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
			);

			expect(onClose).toHaveBeenCalledTimes(1);
			view.unmount();
			expect(document.activeElement).toBe(opener);
		} finally {
			opener.remove();
		}
	});

	it("HUDArticlesTab calls fetchArticlesByTag once on mount and again only when tagName changes", async () => {
		const view = render(HUDArticlesTab, {
			props: {
				tagName: "svelte",
			},
		});

		await expect
			.element(page.getByText("Understanding Svelte 5 Runes"))
			.toBeInTheDocument();
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(1);

		await view.rerender({ tagName: "svelte" });
		expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(1);

		await view.rerender({ tagName: "rust" });
		await vi.waitFor(() => {
			expect(mockFetchArticlesByTag).toHaveBeenCalledTimes(2);
		});
	});
});
