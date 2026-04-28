import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";
import KnowledgeCard from "./KnowledgeCard.svelte";

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

	it("renders title and summary when ready", async () => {
		render(KnowledgeCard as never, {
			props: {
				item: makeItem({
					title: "My Article",
					summaryExcerpt: "Summary text here",
					summaryState: "ready",
				}),
				onAction: vi.fn(),
			},
		});

		await expect.element(page.getByText("My Article")).toBeInTheDocument();
		await expect
			.element(page.getByText("Summary text here"))
			.toBeInTheDocument();
	});

	it("shows skeleton lines when summary_state is pending", async () => {
		const { container } = render(KnowledgeCard as never, {
			props: {
				item: makeItem({
					summaryState: "pending",
					summaryExcerpt: undefined,
				}),
				onAction: vi.fn(),
			},
		});

		// Should show Summarizing chip
		await expect.element(page.getByText("Summarizing")).toBeInTheDocument();

		// Should show pulse skeleton, not text
		const pulseElements = container.querySelectorAll(".animate-pulse");
		expect(pulseElements.length).toBeGreaterThan(0);
	});

	it("shows skeleton lines when summary_state is missing", async () => {
		const { container } = render(KnowledgeCard as never, {
			props: {
				item: makeItem({
					summaryState: "missing",
					summaryExcerpt: undefined,
				}),
				onAction: vi.fn(),
			},
		});

		const pulseElements = container.querySelectorAll(".animate-pulse");
		expect(pulseElements.length).toBeGreaterThan(0);
	});

	it("renders supersede badge when supersedeInfo is present", async () => {
		render(KnowledgeCard as never, {
			props: {
				item: makeItem({
					supersedeInfo: {
						state: "summary_updated",
						supersededAt: "2026-03-17T14:00:00Z",
						previousSummaryExcerpt: "Old summary",
						previousTags: ["OldTag"],
						previousWhyCodes: ["new_unread"],
					},
				}),
				onAction: vi.fn(),
			},
		});

		await expect.element(page.getByText("Summary updated")).toBeInTheDocument();
	});

	it("renders why badges", async () => {
		render(KnowledgeCard as never, {
			props: {
				item: makeItem({
					why: [{ code: "new_unread" }],
				}),
				onAction: vi.fn(),
			},
		});

		await expect.element(page.getByText("New")).toBeInTheDocument();
	});

	it("marks the card link-unavailable when item.url is empty", async () => {
		render(KnowledgeCard as never, {
			props: {
				item: makeItem({ url: undefined }),
				onAction: vi.fn(),
			},
		});

		await expect
			.element(page.getByTestId("kh-card-link-unavailable"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("kh-card-link-unavailable"))
			.toHaveAttribute("aria-disabled", "true");
	});

	it("does not mark the card link-unavailable when item.url is present", async () => {
		const { container } = render(KnowledgeCard as never, {
			props: {
				item: makeItem({ url: "https://example.com/article" }),
				onAction: vi.fn(),
			},
		});

		const flag = container.querySelector(
			"[data-testid='kh-card-link-unavailable']",
		);
		expect(flag).toBeNull();
	});
});
