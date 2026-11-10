import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagCloudItem } from "#lib/connect/index.js";
import TagIndexPanel from "./TagIndexPanel.svelte";

const TAGS: TagCloudItem[] = [
	{
		tagName: "svelte",
		articleCount: 12,
		positionX: 0,
		positionY: 0,
		positionZ: 0,
	},
	{
		tagName: "rust",
		articleCount: 45,
		positionX: 1,
		positionY: 1,
		positionZ: 1,
	},
	{
		tagName: "golang",
		articleCount: 28,
		positionX: -1,
		positionY: 2,
		positionZ: 0,
	},
];

describe("TagIndexPanel", () => {
	it("filters tags by case-insensitive substring via 'Find a tag' search box and calls onSelectTag", async () => {
		const onSelectTag = vi.fn();
		render(TagIndexPanel, {
			props: {
				tags: TAGS,
				selectedTag: "rust",
				onSelectTag,
			},
		});

		const searchInput = page.getByLabelText("Find a tag");
		await expect.element(searchInput).toBeInTheDocument();

		const rustButton = page.getByRole("button", { name: /rust/i });
		await expect.element(rustButton).toHaveAttribute("aria-current", "true");

		await searchInput.fill("SVE");
		await expect
			.element(page.getByRole("button", { name: /svelte/i }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /rust/i }))
			.not.toBeInTheDocument();

		await page.getByRole("button", { name: /svelte/i }).click();
		expect(onSelectTag).toHaveBeenCalledWith("svelte");
	});

	it("toggles index visibility with Hide index / Show index button carrying aria-expanded and aria-controls", async () => {
		render(TagIndexPanel, {
			props: {
				tags: TAGS,
				selectedTag: null,
				onSelectTag: vi.fn(),
			},
		});

		const hideBtn = page.getByRole("button", { name: "Hide index" });
		await expect.element(hideBtn).toHaveAttribute("aria-expanded", "true");
		await expect
			.element(hideBtn)
			.toHaveAttribute("aria-controls", "tag-verse-index-panel");

		await hideBtn.click();

		const showBtn = page.getByRole("button", { name: "Show index" });
		await expect.element(showBtn).toHaveAttribute("aria-expanded", "false");
	});
});
