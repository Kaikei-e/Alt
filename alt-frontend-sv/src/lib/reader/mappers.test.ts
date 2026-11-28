import { describe, expect, it } from "vitest";
import type { KnowledgeHomeItemData } from "#lib/connect/knowledge_home.js";
import { knowledgeHomeItemToReaderItem } from "./mappers.js";

function makeHomeItem(
	overrides: Partial<KnowledgeHomeItemData> = {},
): KnowledgeHomeItemData {
	return {
		itemKey: "article:art-100",
		itemType: "article",
		articleId: "art-100",
		title: "Rust Async Patterns",
		url: "https://example.com/rust-async",
		publishedAt: "2026-11-28T01:00:00Z",
		summaryExcerpt: "Rust の非同期処理における構造化並行性の解説。",
		summaryState: "ready",
		tags: ["rust", "async"],
		why: [],
		score: 0.9,
		...overrides,
	};
}

describe("knowledgeHomeItemToReaderItem", () => {
	it("maps a ready Japanese summary to initialSummary and leaves excerpt/fallbackDescription null", () => {
		const readerItem = knowledgeHomeItemToReaderItem(makeHomeItem());

		expect(readerItem.articleId).toBe("art-100");
		expect(readerItem.url).toBe("https://example.com/rust-async");
		expect(readerItem.title).toBe("Rust Async Patterns");
		expect(readerItem.tagsLabel).toBe("rust · async");
		expect(readerItem.excerpt).toBeNull();
		expect(readerItem.fallbackDescription).toBeNull();
		expect(readerItem.initialSummary).toBe(
			"Rust の非同期処理における構造化並行性の解説。",
		);
	});

	it("omits initialSummary when summaryState is pending or missing", () => {
		const pending = knowledgeHomeItemToReaderItem(
			makeHomeItem({
				summaryState: "pending",
				summaryExcerpt: "stale text that must not render before ready",
			}),
		);
		const missing = knowledgeHomeItemToReaderItem(
			makeHomeItem({
				summaryState: "missing",
				summaryExcerpt: undefined,
			}),
		);

		expect(pending.initialSummary).toBeNull();
		expect(pending.excerpt).toBeNull();
		expect(pending.fallbackDescription).toBeNull();
		expect(missing.initialSummary).toBeNull();
		expect(missing.excerpt).toBeNull();
		expect(missing.fallbackDescription).toBeNull();
	});

	it("trims whitespace and treats blank summaryExcerpt as null", () => {
		const trimmed = knowledgeHomeItemToReaderItem(
			makeHomeItem({
				summaryState: "ready",
				summaryExcerpt: "  日本語要約テキスト  \n",
			}),
		);
		const blank = knowledgeHomeItemToReaderItem(
			makeHomeItem({
				summaryState: "ready",
				summaryExcerpt: "   \n\t  ",
			}),
		);

		expect(trimmed.initialSummary).toBe("日本語要約テキスト");
		expect(blank.initialSummary).toBeNull();
	});
});
