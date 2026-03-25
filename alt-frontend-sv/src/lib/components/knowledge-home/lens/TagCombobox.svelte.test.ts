import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import TagCombobox from "./TagCombobox.svelte";

import type { TagSuggestion } from "./TagCombobox.svelte";

const suggestions: TagSuggestion[] = [
	{ name: "AI", count: 42 },
	{ name: "Rust", count: 15 },
	{ name: "Agents", count: 8 },
	{ name: "Go", count: 30 },
	{ name: "TypeScript", count: 20 },
];

function renderCombobox(overrides: Record<string, unknown> = {}) {
	return render(TagCombobox as never, {
		props: {
			selectedTags: [],
			availableTags: suggestions,
			onTagsChange: vi.fn(),
			...overrides,
		},
	});
}

describe("TagCombobox", () => {
	it("renders input with placeholder", async () => {
		renderCombobox({ placeholder: "Search or add tags..." });
		await expect
			.element(page.getByPlaceholder("Search or add tags..."))
			.toBeInTheDocument();
	});

	it("shows suggestions when input is focused", async () => {
		renderCombobox();
		const input = page.getByRole("combobox");
		await input.click();
		await expect.element(page.getByText("AI")).toBeInTheDocument();
		await expect.element(page.getByText("Rust")).toBeInTheDocument();
	});

	it("filters suggestions by input text", async () => {
		renderCombobox();
		const input = page.getByRole("combobox");
		await input.fill("ru");
		await expect.element(page.getByText("Rust")).toBeInTheDocument();
		await expect.element(page.getByText("AI")).not.toBeInTheDocument();
	});

	it("adds tag on suggestion click and calls onTagsChange", async () => {
		const onTagsChange = vi.fn();
		renderCombobox({ onTagsChange });
		const input = page.getByRole("combobox");
		await input.click();
		await page.getByRole("option", { name: /AI/ }).click();
		expect(onTagsChange).toHaveBeenCalledWith(["AI"]);
	});

	it("renders selected tags as chips", async () => {
		renderCombobox({ selectedTags: ["AI", "Rust"] });
		await expect.element(page.getByText("AI")).toBeInTheDocument();
		await expect.element(page.getByText("Rust")).toBeInTheDocument();
	});

	it("removes tag when chip delete button is clicked", async () => {
		const onTagsChange = vi.fn();
		renderCombobox({ selectedTags: ["AI", "Rust"], onTagsChange });
		const deleteButtons = page.getByRole("button", { name: /remove/i });
		await deleteButtons.first().click();
		expect(onTagsChange).toHaveBeenCalledWith(["Rust"]);
	});

	it("excludes already selected tags from suggestions", async () => {
		renderCombobox({ selectedTags: ["AI"] });
		const input = page.getByRole("combobox");
		await input.click();
		await expect
			.element(page.getByRole("option", { name: /Rust/ }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("option", { name: /^AI/ }))
			.not.toBeInTheDocument();
	});

	it("shows 'Create' option for unmatched input", async () => {
		renderCombobox();
		const input = page.getByRole("combobox");
		await input.fill("NewTag");
		await expect
			.element(page.getByRole("option", { name: /Create "NewTag"/ }))
			.toBeInTheDocument();
	});

	it("displays article count next to suggestion", async () => {
		renderCombobox();
		const input = page.getByRole("combobox");
		await input.click();
		await expect.element(page.getByText("×42")).toBeInTheDocument();
	});

	it("limits suggestions to 20 items", async () => {
		const manyTags: TagSuggestion[] = Array.from({ length: 30 }, (_, i) => ({
			name: `Tag${i}`,
			count: 30 - i,
		}));
		renderCombobox({ availableTags: manyTags });
		const input = page.getByRole("combobox");
		await input.click();
		const options = page.getByRole("option");
		await expect.element(options.nth(19)).toBeInTheDocument();
		// 21st option should not exist (0-indexed, so nth(20))
		await expect.element(options.nth(20)).not.toBeInTheDocument();
	});
});
