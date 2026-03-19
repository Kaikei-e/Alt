import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";
import KnowledgeStream from "./KnowledgeStream.svelte";

function makeItem(
	overrides: Partial<KnowledgeHomeItemData> = {},
): KnowledgeHomeItemData {
	return {
		itemKey: "article:test-1",
		itemType: "article",
		articleId: "test-1",
		title: "Test Article",
		publishedAt: "2026-03-17T10:00:00Z",
		summaryExcerpt: "Test summary",
		summaryState: "ready",
		tags: ["AI"],
		why: [{ code: "new_unread" }],
		score: 0.8,
		...overrides,
	};
}

describe("KnowledgeStream", () => {
	it("renders items as cards", async () => {
		render(KnowledgeStream as never, {
			props: {
				items: [makeItem()],
				loading: false,
				hasMore: false,
				onAction: vi.fn(),
				onLoadMore: vi.fn(),
				onItemsVisible: vi.fn(),
			},
		});

		await expect.element(page.getByText("Test Article")).toBeInTheDocument();
	});

	it("shows skeleton when loading with no items", async () => {
		const { container } = render(KnowledgeStream as never, {
			props: {
				items: [],
				loading: true,
				hasMore: false,
				onAction: vi.fn(),
				onLoadMore: vi.fn(),
				onItemsVisible: vi.fn(),
			},
		});

		const shimmerBlocks = container.querySelectorAll(".animate-shimmer");
		expect(shimmerBlocks.length).toBeGreaterThan(0);
	});

	it("shows empty state with correct reason", async () => {
		render(KnowledgeStream as never, {
			props: {
				items: [],
				loading: false,
				hasMore: false,
				emptyReason: "no_data",
				onAction: vi.fn(),
				onLoadMore: vi.fn(),
				onItemsVisible: vi.fn(),
			},
		});

		await expect.element(page.getByText("No articles yet")).toBeInTheDocument();
	});

	it("shows lens empty state", async () => {
		render(KnowledgeStream as never, {
			props: {
				items: [],
				loading: false,
				hasMore: false,
				emptyReason: "lens_strict",
				activeLensName: "AI News",
				onAction: vi.fn(),
				onLoadMore: vi.fn(),
				onItemsVisible: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("No matches in AI News"))
			.toBeInTheDocument();
	});

	it("shows degraded note when streamMode is provided and degraded", async () => {
		render(KnowledgeStream as never, {
			props: {
				items: [makeItem()],
				loading: false,
				hasMore: false,
				streamMode: "lens",
				degradedNote: "Lens results may be incomplete",
				onAction: vi.fn(),
				onLoadMore: vi.fn(),
				onItemsVisible: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Lens results may be incomplete"))
			.toBeInTheDocument();
	});
});
