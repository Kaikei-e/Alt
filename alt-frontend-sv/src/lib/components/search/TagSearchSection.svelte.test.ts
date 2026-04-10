import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import TagSearchSection from "./TagSearchSection.svelte";
import type { TagSectionData } from "$lib/connect/global_search";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

const sectionFixture: TagSectionData = {
	hits: [
		{ tagName: "artificial-intelligence", articleCount: 128 },
		{ tagName: "machine-learning", articleCount: 64 },
		{ tagName: "svelte", articleCount: 32 },
	],
	total: 3,
};

describe("TagSearchSection Alt-Paper compliance", () => {
	it("renders section with data-role attribute", async () => {
		render(TagSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		const section = document.querySelector(
			"[data-role='reference-tags-section']",
		);
		expect(section).not.toBeNull();
	});

	it("renders TAGS section label", async () => {
		render(TagSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		await expect.element(page.getByText(/TAGS/)).toBeInTheDocument();
		await expect.element(page.getByText("(3)")).toBeInTheDocument();
	});

	it("renders tag buttons with name and count", async () => {
		render(TagSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		await expect
			.element(page.getByText("artificial-intelligence"))
			.toBeInTheDocument();
		await expect.element(page.getByText("(128)")).toBeInTheDocument();
		await expect.element(page.getByText("svelte")).toBeInTheDocument();
	});

	it("shows empty state for no tags", async () => {
		const empty: TagSectionData = { hits: [], total: 0 };

		render(TagSearchSection as never, {
			props: { section: empty, query: "AI" },
		});

		await expect
			.element(page.getByText(/No matching tags/))
			.toBeInTheDocument();
	});
});
