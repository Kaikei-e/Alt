import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import ArticleSearchSection from "./ArticleSearchSection.svelte";
import type { ArticleSectionData } from "$lib/connect/global_search";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

const sectionFixture: ArticleSectionData = {
	hits: [
		{
			id: "a1",
			title: "AI Research Breakthrough",
			snippet: "New model achieves <em>state-of-the-art</em> results",
			link: "https://example.com/ai-research",
			tags: ["AI", "ML"],
			matchedFields: ["title", "content"],
		},
		{
			id: "a2",
			title: "Web Development Trends",
			snippet: "SvelteKit and modern frameworks",
			link: "https://example.com/web-dev",
			tags: ["web"],
			matchedFields: ["title"],
		},
	],
	estimatedTotal: 42,
	hasMore: true,
};

describe("ArticleSearchSection Alt-Paper compliance", () => {
	it("renders section with data-role attribute", async () => {
		render(ArticleSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		const section = document.querySelector(
			"[data-role='reference-articles-section']",
		);
		expect(section).not.toBeNull();
	});

	it("renders ARTICLES section label in uppercase", async () => {
		render(ArticleSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		await expect.element(page.getByText(/ARTICLES/)).toBeInTheDocument();
		await expect.element(page.getByText("(42)")).toBeInTheDocument();
	});

	it("renders see all button with text character instead of icon", async () => {
		render(ArticleSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		await expect.element(page.getByText(/See all/)).toBeInTheDocument();
	});

	it("renders article hit cards", async () => {
		render(ArticleSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		await expect
			.element(page.getByText("AI Research Breakthrough"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Web Development Trends"))
			.toBeInTheDocument();
	});

	it("renders field badges and tag tokens", async () => {
		render(ArticleSearchSection as never, {
			props: { section: sectionFixture, query: "AI" },
		});

		// "content" badge appears only in the first hit's matchedFields
		await expect.element(page.getByText("content")).toBeInTheDocument();
		// "ML" tag appears only in the first hit
		await expect.element(page.getByText("ML")).toBeInTheDocument();
	});

	it("shows empty state with italic text", async () => {
		const emptySection: ArticleSectionData = {
			hits: [],
			estimatedTotal: 0,
			hasMore: false,
		};

		render(ArticleSearchSection as never, {
			props: { section: emptySection, query: "AI" },
		});

		await expect
			.element(page.getByText(/No matching articles/))
			.toBeInTheDocument();
	});
});
