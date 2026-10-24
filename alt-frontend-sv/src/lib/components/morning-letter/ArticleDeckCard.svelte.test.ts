import { page } from "@vitest/browser/context";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";
import type { MorningLetterBulletEnrichment } from "#lib/gen/alt/morning_letter/v2/morning_letter_pb.js";
import ArticleDeckCard from "./ArticleDeckCard.svelte";

function makeEnrichment(
	overrides: Partial<MorningLetterBulletEnrichment> = {},
): MorningLetterBulletEnrichment {
	return {
		$typeName: "alt.morning_letter.v2.MorningLetterBulletEnrichment",
		sectionKey: "top3",
		articleId: "art-101",
		articleTitle: "Distributed Consensus in 2026",
		articleUrl: "https://example.com/consensus-2026",
		articleAltHref: "/articles/art-101",
		feedTitle: "Systems Weekly",
		tags: ["distributed-systems", "raft", "paxos", "storage", "go", "rust"],
		relatedArticles: [],
		summaryExcerpt: "New consensus variants reduce tail latency across WANs.",
		chatHref: "/augur?articleId=art-101",
		...overrides,
	};
}

describe("ArticleDeckCard", () => {
	it("caps displayed tags at 4 and renders +N overflow badge", async () => {
		const { container } = render(ArticleDeckCard, {
			props: {
				enrichment: makeEnrichment({
					tags: ["tag1", "tag2", "tag3", "tag4", "tag5", "tag6"],
				}),
			},
		});

		await expect.element(page.getByText("#tag1")).toBeInTheDocument();
		await expect.element(page.getByText("#tag2")).toBeInTheDocument();
		await expect.element(page.getByText("#tag3")).toBeInTheDocument();
		await expect.element(page.getByText("#tag4")).toBeInTheDocument();
		expect(container.textContent).not.toContain("#tag5");
		expect(container.textContent).not.toContain("#tag6");

		const overflow = container.querySelector(".deck-tag--more");
		expect(overflow).not.toBeNull();
		expect(overflow?.textContent?.trim()).toBe("+2");
	});

	it("renders internal /articles/<id> link when articleAltHref is provided", async () => {
		render(ArticleDeckCard, {
			props: {
				enrichment: makeEnrichment({
					articleTitle: "Internal Reader Target",
					articleAltHref: "/articles/art-101",
				}),
			},
		});

		const link = page.getByRole("link", { name: "Internal Reader Target" });
		await expect.element(link).toBeInTheDocument();
		await expect.element(link).toHaveAttribute("href", "/articles/art-101");
	});

	it("renders safe https external Original link", async () => {
		render(ArticleDeckCard, {
			props: {
				enrichment: makeEnrichment({
					articleUrl: "https://example.com/safe-article",
				}),
			},
		});

		const originalLink = page.getByRole("link", { name: /original/i });
		await expect.element(originalLink).toBeInTheDocument();
		await expect
			.element(originalLink)
			.toHaveAttribute("href", "https://example.com/safe-article");
	});

	it("rejects javascript: and data: URLs in enrichment.articleUrl", async () => {
		const { container, unmount } = render(ArticleDeckCard, {
			props: {
				enrichment: makeEnrichment({
					articleUrl: "javascript:alert(1)",
				}),
			},
		});

		expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
		expect(container.textContent).not.toContain("Original ↗");
		unmount();

		const { container: dataContainer } = render(ArticleDeckCard, {
			props: {
				enrichment: makeEnrichment({
					articleUrl: "data:text/html,<script>alert(1)</script>",
				}),
			},
		});

		expect(dataContainer.querySelector('a[href^="data:"]')).toBeNull();
		expect(dataContainer.textContent).not.toContain("Original ↗");
	});
});
