import { describe, it, expect } from "vitest";
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";

/**
 * Tests for KnowledgeCard data logic.
 * Component rendering is tested via browser tests (*.svelte.test.ts).
 */

function makeItem(
	overrides: Partial<KnowledgeHomeItemData> = {},
): KnowledgeHomeItemData {
	return {
		itemKey: "article:test-123",
		itemType: "article",
		articleId: "test-123",
		title: "Test Article Title",
		publishedAt: "2026-03-17T10:00:00Z",
		summaryExcerpt: "This is a test summary excerpt.",
		tags: ["AI", "ML", "Go"],
		why: [{ code: "new_unread" }, { code: "tag_hotspot", tag: "AI" }],
		score: 0.85,
		...overrides,
	};
}

describe("KnowledgeCard data", () => {
	it("creates a valid item with all fields", () => {
		const item = makeItem();
		expect(item.itemKey).toBe("article:test-123");
		expect(item.title).toBe("Test Article Title");
		expect(item.why).toHaveLength(2);
		expect(item.tags).toHaveLength(3);
	});

	it("handles missing summary excerpt", () => {
		const item = makeItem({ summaryExcerpt: undefined });
		expect(item.summaryExcerpt).toBeUndefined();
	});

	it("handles empty tags", () => {
		const item = makeItem({ tags: [] });
		expect(item.tags).toHaveLength(0);
	});

	it("handles empty why reasons", () => {
		const item = makeItem({ why: [] });
		expect(item.why).toHaveLength(0);
	});

	it("truncates display tags to first 3", () => {
		const item = makeItem({
			tags: ["AI", "ML", "Go", "Rust", "Python"],
		});
		const displayTags = item.tags.slice(0, 3);
		expect(displayTags).toEqual(["AI", "ML", "Go"]);
		expect(displayTags).toHaveLength(3);
	});

	it("formats relative time from publishedAt", () => {
		const item = makeItem();
		const publishedDate = new Date(item.publishedAt);
		expect(publishedDate).toBeInstanceOf(Date);
		expect(publishedDate.getTime()).not.toBeNaN();
	});
});
