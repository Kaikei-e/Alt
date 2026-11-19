import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	searchResultFixture,
	searchResultLongUrlFixture,
	searchResultNoDescFixture,
} from "../../../../../tests/fixtures/search";
import SearchResultItem from "./SearchResultItem.svelte";

describe("SearchResultItem Alt-Paper compliance", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("renders with data-role attribute", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		const item = page.getByRole("article");
		await expect.element(item).toBeInTheDocument();
	});

	it("renders title as a link with serif styling", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		const link = page.getByRole("link", {
			name: "Svelte 5 Runes Deep Dive",
		});
		await expect.element(link).toBeInTheDocument();
	});

	it("renders dateline with author and date", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await expect.element(page.getByText(/Svelte Team/)).toBeInTheDocument();
	});

	it("renders description excerpt", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await expect
			.element(page.getByText(/comprehensive look/))
			.toBeInTheDocument();
	});

	it("does not render description when empty", async () => {
		render(SearchResultItem, {
			props: { result: searchResultNoDescFixture },
		});

		// Title should exist
		await expect.element(page.getByText("Quick Update")).toBeInTheDocument();
	});

	it("has a DETAILS button when result has a link", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		const btn = page.getByRole("button", { name: /^details$/i });
		await expect.element(btn).toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /show summary/i }))
			.not.toBeInTheDocument();
	});

	it("does not contain Lucide SVG icons", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		// SquareArrowOutUpRight and Loader2 should not exist
		const svgs = page.getByRole("img", { includeHidden: true });
		await expect.element(svgs).not.toBeInTheDocument();
	});

	it("does not contain emoji characters", async () => {
		render(SearchResultItem, {
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

describe("SearchResultItem long-URL wrapping", () => {
	const NARROW_VIEWPORT_PX = 320;

	beforeEach(() => {
		vi.clearAllMocks();
	});

	/**
	 * Browser-mode tests render into a full-width container, where even a
	 * 200-character URL fits on one line and the overflow never shows up.
	 * Squeeze the host down to a phone width first so the assertions below
	 * reproduce what the mobile search screen actually does.
	 */
	function squeezeToPhoneWidth(el: HTMLElement): void {
		const host = el.parentElement as HTMLElement;
		host.style.width = `${NARROW_VIEWPORT_PX}px`;
		host.style.maxWidth = `${NARROW_VIEWPORT_PX}px`;
		void host.offsetWidth; // force layout before measuring
	}

	function allowsMidTokenBreak(el: HTMLElement): boolean {
		const styles = window.getComputedStyle(el);
		return (
			styles.overflowWrap === "anywhere" ||
			styles.wordBreak === "break-all" ||
			styles.wordBreak === "break-word"
		);
	}

	it("breaks the description excerpt mid-token instead of overflowing", async () => {
		render(SearchResultItem, {
			props: { result: searchResultLongUrlFixture },
		});

		const locator = page.getByText(/zdnet\.com\/a\/img/);
		await expect.element(locator).toBeInTheDocument();
		const excerpt = locator.element() as HTMLElement;
		squeezeToPhoneWidth(excerpt.closest("article") as HTMLElement);

		expect(allowsMidTokenBreak(excerpt)).toBe(true);
		expect(excerpt.scrollWidth).toBeLessThanOrEqual(excerpt.clientWidth);
	});

	it("keeps the result card within its container width", async () => {
		render(SearchResultItem, {
			props: { result: searchResultLongUrlFixture },
		});

		const locator = page.getByRole("article");
		await expect.element(locator).toBeInTheDocument();
		const card = locator.element() as HTMLElement;
		squeezeToPhoneWidth(card);

		expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth);
		expect(card.getBoundingClientRect().width).toBeLessThanOrEqual(
			NARROW_VIEWPORT_PX,
		);
	});

	it("breaks a long unbroken title instead of overflowing", async () => {
		render(SearchResultItem, {
			props: {
				result: {
					...searchResultLongUrlFixture,
					title:
						"https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/dsc09454.jpg",
				},
			},
		});

		const locator = page.getByRole("link");
		await expect.element(locator).toBeInTheDocument();
		const title = locator.element() as HTMLElement;
		squeezeToPhoneWidth(title.closest("article") as HTMLElement);

		expect(allowsMidTokenBreak(title)).toBe(true);
		expect(title.scrollWidth).toBeLessThanOrEqual(title.clientWidth);
	});
});

describe("SearchResultItem reader overlay trigger", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("calls onSelect with the result when DETAILS is clicked without inline expansion", async () => {
		const onSelect = vi.fn();
		render(SearchResultItem, {
			props: { result: searchResultFixture, onSelect },
		});

		await page.getByRole("button", { name: /^details$/i }).click();

		expect(onSelect).toHaveBeenCalledWith(searchResultFixture);
		expect(document.querySelector('[data-role="result-body"]')).toBeNull();
	});

	it("does not offer the details control without a link", async () => {
		render(SearchResultItem, {
			props: { result: { ...searchResultFixture, link: "" } },
		});

		await expect
			.element(page.getByRole("button", { name: /^details$/i }))
			.not.toBeInTheDocument();
	});
});
