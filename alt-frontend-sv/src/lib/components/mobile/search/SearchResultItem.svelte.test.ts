import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SearchResultItem from "./SearchResultItem.svelte";
import {
	searchResultFixture,
	searchResultLongUrlFixture,
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
	getFeedContentOnTheFlyClient: vi.fn(() =>
		Promise.resolve({
			content: "<p>Full article body paragraph.</p>",
			article_id: "article-123",
			og_image_url: "",
			og_image_proxy_url: "",
		}),
	),
}));

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

	it("has toggle summary button with uppercase text", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		const btn = page.getByRole("button", { name: /show summary/i });
		await expect.element(btn).toBeInTheDocument();
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

	it("breaks long URLs inside an expanded summary", async () => {
		const { getArticleSummaryClient } = await import("$lib/api/client");
		vi.mocked(getArticleSummaryClient).mockResolvedValueOnce({
			matched_articles: [
				{
					title: "Summary Title",
					content:
						"See https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/77bc4efe-edc8-4a95-8fc0-d630a7f6f1c9/dsc09454.jpg?auto=webp&fit=crop&height=675&width=1200",
				},
			],
			// biome-ignore lint/suspicious/noExplicitAny: partial API shape is enough here
		} as any);

		render(SearchResultItem, {
			props: { result: searchResultLongUrlFixture },
		});

		await page.getByRole("button", { name: /show summary/i }).click();

		// Scoped to the summary: the excerpt on this fixture also contains
		// "auto=webp", and an unscoped match is a strict-mode violation.
		const locator = page.getByText(/^See https:/);
		await expect.element(locator).toBeInTheDocument();
		const prose = locator.element() as HTMLElement;
		squeezeToPhoneWidth(prose.closest("article") as HTMLElement);

		expect(allowsMidTokenBreak(prose)).toBe(true);
		expect(prose.scrollWidth).toBeLessThanOrEqual(prose.clientWidth);
	});
});

describe("SearchResultItem article body on demand", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	async function contentClient() {
		const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
		return vi.mocked(getFeedContentOnTheFlyClient);
	}

	it("offers a details control alongside the summary control", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await expect
			.element(page.getByRole("button", { name: /^details$/i }))
			.toBeInTheDocument();
	});

	it("does not fetch the body until the control is tapped", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await expect
			.element(page.getByRole("button", { name: /^details$/i }))
			.toBeInTheDocument();
		expect(await contentClient()).not.toHaveBeenCalled();
	});

	it("fetches and renders the article body when tapped", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await page.getByRole("button", { name: /^details$/i }).click();

		await expect
			.element(page.getByText("Full article body paragraph."))
			.toBeInTheDocument();
		expect(await contentClient()).toHaveBeenCalledWith(
			searchResultFixture.link,
		);
	});

	it("collapses the body on the second tap without refetching", async () => {
		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await page.getByRole("button", { name: /^details$/i }).click();
		await expect
			.element(page.getByText("Full article body paragraph."))
			.toBeInTheDocument();

		await page.getByRole("button", { name: /hide details/i }).click();
		await expect
			.element(page.getByText("Full article body paragraph."))
			.not.toBeInTheDocument();

		await page.getByRole("button", { name: /^details$/i }).click();
		await expect
			.element(page.getByText("Full article body paragraph."))
			.toBeInTheDocument();
		expect(await contentClient()).toHaveBeenCalledTimes(1);
	});

	it("surfaces a retryable failure instead of an empty panel", async () => {
		(await contentClient()).mockRejectedValueOnce(new Error("boom"));

		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await page.getByRole("button", { name: /^details$/i }).click();

		await expect
			.element(page.getByText(/Source content unavailable/i))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /try again/i }))
			.toBeInTheDocument();
	});

	it("treats an empty body as a stated failure, not as success", async () => {
		(await contentClient()).mockResolvedValueOnce({
			content: "",
			article_id: "article-123",
			og_image_url: "",
			og_image_proxy_url: "",
		});

		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await page.getByRole("button", { name: /^details$/i }).click();

		await expect
			.element(page.getByText(/Source content unavailable/i))
			.toBeInTheDocument();
	});

	it("wraps long URLs inside the fetched body", async () => {
		(await contentClient()).mockResolvedValueOnce({
			content:
				'<p>See <a href="https://example.com/x">https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/77bc4efe-edc8-4a95-8fc0-d630a7f6f1c9/dsc09454.jpg?auto=webp&amp;fit=crop&amp;height=675&amp;width=1200</a></p>',
			article_id: "article-123",
			og_image_url: "",
			og_image_proxy_url: "",
		});

		render(SearchResultItem, {
			props: { result: searchResultFixture },
		});

		await page.getByRole("button", { name: /^details$/i }).click();

		const locator = page.getByText(/auto=webp/);
		await expect.element(locator).toBeInTheDocument();
		const link = locator.element() as HTMLElement;
		const card = link.closest("article") as HTMLElement;
		const host = card.parentElement as HTMLElement;
		host.style.width = "320px";
		host.style.maxWidth = "320px";
		void host.offsetWidth;

		expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth);
	});

	it("fits both controls on one row at phone width", async () => {
		render(SearchResultItem, {
			props: { result: searchResultLongUrlFixture },
		});

		const details = page
			.getByRole("button", { name: /^details$/i })
			.element() as HTMLElement;
		const row = details.parentElement as HTMLElement;
		const host = (details.closest("article") as HTMLElement)
			.parentElement as HTMLElement;
		host.style.width = "320px";
		host.style.maxWidth = "320px";
		void host.offsetWidth;

		// Two 44px-tall bordered controls sharing one row is the tightest thing
		// on this card. Neither may spill its own label nor widen the row.
		expect(row.scrollWidth).toBeLessThanOrEqual(row.clientWidth);
		for (const btn of Array.from(row.children) as HTMLElement[]) {
			expect(btn.scrollWidth).toBeLessThanOrEqual(btn.clientWidth);
		}
	});

	it("does not offer the details control without a link", async () => {
		render(SearchResultItem, {
			props: { result: { ...searchResultFixture, link: "" } },
		});

		await expect
			.element(page.getByRole("button", { name: /show summary/i }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /^details$/i }))
			.not.toBeInTheDocument();
	});
});
