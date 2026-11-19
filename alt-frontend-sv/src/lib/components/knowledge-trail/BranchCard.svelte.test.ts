import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { BranchData } from "#lib/connect/knowledge_trail.js";
import BranchCard from "./BranchCard.svelte";

const NARROW_VIEWPORT_PX = 375;
const LONG_URL =
	"https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/77bc4efe8fc0d630a7f6f1c9dsc09454.jpg?auto=webp&fit=crop&height=675&width=1200";

function makeBranch(overrides: Partial<BranchData> = {}): BranchData {
	return {
		branchKey: "continuation:u:article:z",
		anchorItemKey: "article:anchor",
		relationKind: "continuation",
		why: "Continues a thread you started.",
		evidenceRefs: [{ refId: "src", label: "shared source", kind: "source" }],
		confidence: "high",
		targetItemKey: "article:z",
		targetTitle: "Next article",
		...overrides,
	};
}

function allowsMidTokenBreak(el: HTMLElement): boolean {
	const styles = window.getComputedStyle(el);
	return (
		styles.overflowWrap === "anywhere" ||
		styles.wordBreak === "break-all" ||
		styles.wordBreak === "break-word"
	);
}

describe("BranchCard long URL wrapping", () => {
	it("wraps a long URL in the title, why, and evidence without truncating", async () => {
		render(BranchCard, {
			props: {
				branch: makeBranch({
					targetTitle: LONG_URL,
					why: LONG_URL,
					evidenceRefs: [{ refId: "src", label: LONG_URL, kind: "source" }],
				}),
				testId: "branch-card",
				onResolve: vi.fn(),
			},
		});

		const locator = page.getByTestId("branch-card");
		await expect.element(locator).toBeInTheDocument();
		const card = locator.element() as HTMLElement;
		const host = card.parentElement as HTMLElement;
		host.style.width = `${NARROW_VIEWPORT_PX}px`;
		host.style.maxWidth = `${NARROW_VIEWPORT_PX}px`;
		host.style.boxSizing = "border-box";
		void host.offsetWidth;

		const title = card.querySelector(".branch-title") as HTMLElement;
		const why = card.querySelector(".branch-why") as HTMLElement;
		const evidence = card.querySelector(".branch-evidence") as HTMLElement;

		for (const el of [title, why, evidence]) {
			expect(allowsMidTokenBreak(el)).toBe(true);
			expect(window.getComputedStyle(el).textOverflow).not.toBe("ellipsis");
			expect(el.textContent).toContain(LONG_URL);
			expect(el.scrollWidth).toBeLessThanOrEqual(el.clientWidth);
		}
		expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth);
		expect(card.getBoundingClientRect().width).toBeLessThanOrEqual(
			NARROW_VIEWPORT_PX,
		);
	});
});
