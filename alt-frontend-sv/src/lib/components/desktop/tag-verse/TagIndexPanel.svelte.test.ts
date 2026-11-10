import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagCloudItem } from "#lib/connect/index.js";
import TagIndexPanel from "./TagIndexPanel.svelte";

const TAGS: TagCloudItem[] = [
	{
		tagName: "svelte",
		articleCount: 1,
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
	it("renders tag buttons inside ul/li with accessible names including article count and filters via search box", async () => {
		const onSelectTag = vi.fn();
		const { container } = render(TagIndexPanel, {
			props: {
				tags: TAGS,
				selectedTag: "rust",
				onSelectTag,
			},
		});

		const searchInput = page.getByLabelText("Find a tag");
		await expect.element(searchInput).toBeInTheDocument();

		expect(container.querySelectorAll("ul > li").length).toBe(3);

		const rustButton = page.getByRole("button", { name: "rust, 45 articles" });
		await expect
			.element(rustButton, { timeout: 1000 })
			.toHaveAttribute("aria-current", "true");

		const svelteButton = page.getByRole("button", {
			name: "svelte, 1 article",
		});
		await expect.element(svelteButton, { timeout: 1000 }).toBeInTheDocument();

		await searchInput.fill("SVE");
		await expect.element(svelteButton, { timeout: 1000 }).toBeInTheDocument();
		await expect
			.element(rustButton, { timeout: 1000 })
			.not.toBeInTheDocument();

		await svelteButton.click();
		expect(onSelectTag).toHaveBeenCalledWith("svelte");
	});

	it("shows 'No matching tags.' only when a non-empty search filters everything out, not when tags list is empty", async () => {
		const { container, rerender } = render(TagIndexPanel, {
			props: {
				tags: [],
				selectedTag: null,
				onSelectTag: vi.fn(),
			},
		});

		expect(container.textContent).not.toContain("No matching tags.");

		await rerender({
			tags: TAGS,
			selectedTag: null,
			onSelectTag: vi.fn(),
		});

		const searchInput = page.getByLabelText("Find a tag");
		await searchInput.fill("nonexistent-tag");
		expect(container.textContent).toContain("No matching tags.");
	});

	it("toggles index visibility with Hide index / Show index button carrying aria-expanded and matching aria-controls", async () => {
		const { container } = render(TagIndexPanel, {
			props: {
				tags: TAGS,
				selectedTag: null,
				onSelectTag: vi.fn(),
			},
		});

		const hideBtn = page.getByRole("button", { name: "Hide index" });
		await expect.element(hideBtn).toHaveAttribute("aria-expanded", "true");
		const hideEl = await hideBtn.element();
		const controlsId = hideEl.getAttribute("aria-controls");
		expect(controlsId).toBeTruthy();
		expect(container.querySelector(`#${controlsId}`)).not.toBeNull();

		await hideBtn.click();

		const showBtn = page.getByRole("button", { name: "Show index" });
		await expect.element(showBtn).toHaveAttribute("aria-expanded", "false");
	});
});
