import { describe, expect, it } from "vitest";
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";
import { buildHomeActionMetadata } from "./home-actions";

function makeItem(
	overrides: Partial<KnowledgeHomeItemData> = {},
): KnowledgeHomeItemData {
	return {
		itemKey: "article:123",
		itemType: "article",
		articleId: "123",
		title: "Maestro MCP と Claude で Android アプリの E2E テストを実装してみた",
		publishedAt: "2026-04-16T00:00:00Z",
		summaryExcerpt:
			"記事要約がここに入る。タイトル同様、body へ載せると WAF の反応面を広げる。",
		summaryState: "ready",
		tags: [],
		why: [],
		score: 0.5,
		...overrides,
	};
}

describe("buildHomeActionMetadata", () => {
	it.each([
		"dismiss",
		"open",
		"open_recap",
		"open_search",
		"ask",
		"listen",
	])("returns undefined for %s so article content never reaches the POST body", (type) => {
		expect(buildHomeActionMetadata(type, makeItem())).toBeUndefined();
	});

	it("does not leak article title even when it contains sensitive keywords", () => {
		const item = makeItem({ title: "Maestro MCP / Claude bypass test" });
		const result = buildHomeActionMetadata("dismiss", item);
		expect(result).toBeUndefined();
	});

	it("does not leak summaryExcerpt even when populated", () => {
		const item = makeItem({
			summaryExcerpt: "contains OWASP-CRS trigger words",
		});
		const result = buildHomeActionMetadata("dismiss", item);
		expect(result).toBeUndefined();
	});
});
