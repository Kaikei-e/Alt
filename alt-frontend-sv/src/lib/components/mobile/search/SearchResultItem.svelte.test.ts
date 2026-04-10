import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SearchResultItem from "./SearchResultItem.svelte";
import {
	searchResultFixture,
	searchResultNoDescFixture,
} from "../../../../../tests/fixtures/search";

vi.mock("$lib/api/client", () => ({
	getArticleSummaryClient: vi.fn(() =>
		Promise.resolve({
			matched_articles: [
				{ title: "Summary Title", content: "Summary content here." },
			],
		}),
	),
}));

describe("SearchResultItem Alt-Paper compliance", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("renders with data-role attribute", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		const item = page.getByRole("article");
		await expect.element(item).toBeInTheDocument();
	});

	it("renders title as a link with serif styling", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		const link = page.getByRole("link", {
			name: "Svelte 5 Runes Deep Dive",
		});
		await expect.element(link).toBeInTheDocument();
	});

	it("renders dateline with author and date", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		await expect.element(page.getByText(/Svelte Team/)).toBeInTheDocument();
	});

	it("renders description excerpt", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		await expect
			.element(page.getByText(/comprehensive look/))
			.toBeInTheDocument();
	});

	it("does not render description when empty", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultNoDescFixture },
		});

		// Title should exist
		await expect.element(page.getByText("Quick Update")).toBeInTheDocument();
	});

	it("has toggle summary button with uppercase text", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		const btn = page.getByRole("button", { name: /show summary/i });
		await expect.element(btn).toBeInTheDocument();
	});

	it("does not contain Lucide SVG icons", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		// SquareArrowOutUpRight and Loader2 should not exist
		const svgs = page.getByRole("img", { includeHidden: true });
		await expect.element(svgs).not.toBeInTheDocument();
	});

	it("does not contain emoji characters", async () => {
		render(SearchResultItem as never, {
			props: { result: searchResultFixture },
		});

		// No sparkle emoji in any button text
		const container = document.querySelector(
			"[data-role='archive-result-item']",
		);
		if (container) {
			expect(container.textContent).not.toContain("\u2728");
			expect(container.textContent).not.toContain("\uD83D\uDD0D");
		}
	});
});
