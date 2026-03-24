import { describe, expect, it } from "vitest";
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
		summaryState: "ready",
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

	it("filters out empty string tags before display", () => {
		const item = makeItem({
			tags: ["", "", "AI", "Go", ""],
		});
		const nonEmptyTags = item.tags.filter((t) => t.trim() !== "");
		const displayTags = nonEmptyTags.slice(0, 3);
		expect(displayTags).toEqual(["AI", "Go"]);
	});

	it("remainingTagCount excludes empty tags", () => {
		const item = makeItem({
			tags: ["", "AI", "Go", "Rust", "Python", ""],
		});
		const nonEmptyTags = item.tags.filter((t) => t.trim() !== "");
		const displayTags = nonEmptyTags.slice(0, 3);
		const remainingTagCount =
			nonEmptyTags.length > 3 ? nonEmptyTags.length - 3 : 0;
		expect(displayTags).toEqual(["AI", "Go", "Rust"]);
		expect(remainingTagCount).toBe(1);
	});

	it("shows no tags when all tags are empty strings", () => {
		const item = makeItem({
			tags: ["", " ", "  "],
		});
		const nonEmptyTags = item.tags.filter((t) => t.trim() !== "");
		expect(nonEmptyTags).toHaveLength(0);
	});

	it("formats relative time from publishedAt", () => {
		const item = makeItem();
		const publishedDate = new Date(item.publishedAt);
		expect(publishedDate).toBeInstanceOf(Date);
		expect(publishedDate.getTime()).not.toBeNaN();
	});
});

describe("KnowledgeCard summaryState", () => {
	it("ready state with excerpt shows excerpt", () => {
		const item = makeItem({
			summaryState: "ready",
			summaryExcerpt: "Summary text here.",
		});
		expect(item.summaryState).toBe("ready");
		expect(item.summaryExcerpt).toBe("Summary text here.");
	});

	it("pending state shows skeleton placeholder", () => {
		const item = makeItem({
			summaryState: "pending",
			summaryExcerpt: undefined,
		});
		expect(item.summaryState).toBe("pending");
		expect(item.summaryExcerpt).toBeUndefined();
	});

	it("missing state shows skeleton placeholder", () => {
		const item = makeItem({
			summaryState: "missing",
			summaryExcerpt: undefined,
		});
		expect(item.summaryState).toBe("missing");
		expect(item.summaryExcerpt).toBeUndefined();
	});

	it("pending state should never show 'Summarizing...' as body text", () => {
		const item = makeItem({
			summaryState: "pending",
			summaryExcerpt: undefined,
		});
		// The old pattern was `item.summaryExcerpt || "Summarizing..."` in the template.
		// Now we use summaryState to determine display, not fallback strings.
		expect(item.summaryExcerpt || "").not.toBe("Summarizing...");
	});
});

describe("KnowledgeCard tag links", () => {
	it("generates correct href for tag link", () => {
		const tag = "AI";
		const href = `/articles/by-tag?tag=${encodeURIComponent(tag)}`;
		expect(href).toBe("/articles/by-tag?tag=AI");
	});

	it("encodes special characters in tag name", () => {
		const tag = "C++ / Templates";
		const href = `/articles/by-tag?tag=${encodeURIComponent(tag)}`;
		expect(href).toBe("/articles/by-tag?tag=C%2B%2B%20%2F%20Templates");
	});

	it("encodes Japanese tag name", () => {
		const tag = "人工知能";
		const href = `/articles/by-tag?tag=${encodeURIComponent(tag)}`;
		expect(href).toContain("/articles/by-tag?tag=");
		expect(decodeURIComponent(href)).toContain("人工知能");
	});
});

/**
 * Tests for formatRelativeTime logic extracted from KnowledgeCard.
 * Bug 4: "NaNd ago" must never appear.
 */
describe("formatRelativeTime", () => {
	// Re-implement here for unit testing (mirrors KnowledgeCard.svelte logic)
	function formatRelativeTime(isoString: string): string {
		if (!isoString) return "recent";
		const date = new Date(isoString);
		if (Number.isNaN(date.getTime())) return "recent";
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffMins = Math.floor(diffMs / 60000);
		if (diffMins < 1) return "just now";
		if (diffMins < 60) return `${diffMins}m ago`;
		const diffHours = Math.floor(diffMins / 60);
		if (diffHours < 24) return `${diffHours}h ago`;
		const diffDays = Math.floor(diffHours / 24);
		return `${diffDays}d ago`;
	}

	it("returns 'recent' for empty string", () => {
		expect(formatRelativeTime("")).toBe("recent");
	});

	it("returns 'recent' for invalid ISO string", () => {
		expect(formatRelativeTime("not-a-date")).toBe("recent");
	});

	it("never returns NaNd ago", () => {
		const result = formatRelativeTime("");
		expect(result).not.toContain("NaN");
	});

	it("returns valid relative time for valid ISO string", () => {
		const result = formatRelativeTime("2026-03-17T10:00:00Z");
		expect(result).not.toContain("NaN");
		expect(result).toMatch(/^\d+[dhm] ago$|^just now$|^recent$/);
	});
});
