import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { FootprintData } from "#lib/connect/knowledge_trail.js";
import Footprint from "./Footprint.svelte";

function makeFootprint(overrides: Partial<FootprintData> = {}): FootprintData {
	return {
		footprintKey: "open:article:abc",
		verb: "read",
		itemKey: "article:11111111-2222-3333-4444-555555555555",
		title: "Hunting Submarines",
		excerpt: "An article about gravity.",
		tags: ["submarine"],
		note: "",
		occurredAt: "2026-06-11T13:00:00Z",
		wear: "thin",
		contactCount: 1,
		firstOccurredAt: "2026-06-11T13:00:00Z",
		...overrides,
	};
}

describe("Footprint", () => {
	it("links the title to the in-app article reader by id only (no url)", async () => {
		render(Footprint, { props: { footprint: makeFootprint() } });
		const link = page.getByTestId("footprint-link");
		await expect.element(link).toBeInTheDocument();
		// id-only: the article id (after the `article:` prefix), no `?url=`.
		await expect
			.element(link)
			.toHaveAttribute(
				"href",
				"/articles/11111111-2222-3333-4444-555555555555",
			);
	});

	it("renders a plain title (no link) for non-article items", async () => {
		render(Footprint, {
			props: { footprint: makeFootprint({ itemKey: "digest:2026-06-11" }) },
		});
		expect(page.getByTestId("footprint-link").elements()).toHaveLength(0);
	});

	// D24: repeated contacts collapse server-side into one row with a count.
	it("shows a visit count when contacts are collapsed", async () => {
		render(Footprint, {
			props: {
				footprint: makeFootprint({
					contactCount: 3,
					firstOccurredAt: "2026-06-01T08:00:00Z",
				}),
			},
		});
		const count = page.getByTestId("footprint-count");
		await expect.element(count).toBeInTheDocument();
		await expect.element(count).toHaveTextContent("3");
	});

	it("shows no visit count for a single contact", async () => {
		render(Footprint, { props: { footprint: makeFootprint() } });
		expect(page.getByTestId("footprint-count").elements()).toHaveLength(0);
	});
});

describe("Footprint long URL wrapping", () => {
	const NARROW_VIEWPORT_PX = 375;
	// No hyphens: Chrome breaks after "-" even without overflow-wrap, which hides
	// the bug. This matches the unbreakable CDN tokens search already guards.
	const LONG_URL =
		"https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/77bc4efe8fc0d630a7f6f1c9dsc09454.jpg?auto=webp&fit=crop&height=675&width=1200";

	function squeezeToPhoneWidth(el: HTMLElement): void {
		const host = el.parentElement as HTMLElement;
		host.style.width = `${NARROW_VIEWPORT_PX}px`;
		host.style.maxWidth = `${NARROW_VIEWPORT_PX}px`;
		host.style.boxSizing = "border-box";
		void host.offsetWidth;
	}

	function allowsMidTokenBreak(el: HTMLElement): boolean {
		const styles = window.getComputedStyle(el);
		return (
			styles.overflowWrap === "anywhere" ||
			styles.wordBreak === "break-all" ||
			styles.wordBreak === "break-word"
		);
	}

	function expectWrapped(el: HTMLElement, root: HTMLElement): void {
		expect(root.clientWidth).toBeGreaterThan(0);
		expect(allowsMidTokenBreak(el)).toBe(true);
		expect(window.getComputedStyle(el).whiteSpace).not.toBe("nowrap");
		expect(window.getComputedStyle(el).textOverflow).not.toBe("ellipsis");
		expect(el.textContent).toContain(LONG_URL);
		expect(el.scrollWidth).toBeLessThanOrEqual(el.clientWidth);
		expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth);
		expect(root.getBoundingClientRect().width).toBeLessThanOrEqual(
			NARROW_VIEWPORT_PX,
		);
	}

	it("wraps a long unbroken title URL inside the article link", async () => {
		render(Footprint, {
			props: { footprint: makeFootprint({ title: LONG_URL, excerpt: "" }) },
		});
		const locator = page.getByTestId("footprint-link");
		await expect.element(locator).toBeInTheDocument();
		const link = locator.element() as HTMLElement;
		const root = link.closest(".step") as HTMLElement;
		squeezeToPhoneWidth(root);
		expectWrapped(link, root);
	});

	it("wraps a long URL in the excerpt without truncating it", async () => {
		render(Footprint, {
			props: {
				footprint: makeFootprint({ title: "Short title", excerpt: LONG_URL }),
			},
		});
		const locator = page.getByText(LONG_URL);
		await expect.element(locator).toBeInTheDocument();
		const excerpt = locator.element() as HTMLElement;
		const root = excerpt.closest(".step") as HTMLElement;
		squeezeToPhoneWidth(root);
		expectWrapped(excerpt, root);
	});

	it("wraps a long item key shown as text when the row is not an article link", async () => {
		render(Footprint, {
			props: {
				footprint: makeFootprint({
					itemKey: LONG_URL,
					title: "",
					excerpt: "",
					note: LONG_URL,
				}),
			},
		});
		const root = page.getByTestId("trail-footprint").element() as HTMLElement;
		squeezeToPhoneWidth(root);
		const title = root.querySelector(".fp-title") as HTMLElement;
		const note = root.querySelector(".fp-note") as HTMLElement;
		expectWrapped(title, root);
		expectWrapped(note, root);
	});
});
