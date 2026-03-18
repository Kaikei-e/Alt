import { render } from "vitest-browser-svelte";
import { page } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import KnowledgeCard from "./KnowledgeCard.svelte";
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";

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
		tags: ["AI", "ML", "Go", "Rust", "Python"],
		why: [{ code: "new_unread" }, { code: "tag_hotspot", tag: "AI" }],
		score: 0.85,
		...overrides,
	};
}

describe("KnowledgeCard", () => {
	it("labels overflow tags explicitly", async () => {
		render(KnowledgeCard as never, {
			props: {
				item: makeItem(),
				onAction: vi.fn(),
			},
		});

		await expect.element(page.getByText("+2 tags")).toBeInTheDocument();
	});
});
