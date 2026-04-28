import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";
import RecallCandidateCard from "./RecallCandidateCard.svelte";

function makeCandidate(
	overrides: Partial<RecallCandidateData> = {},
): RecallCandidateData {
	return {
		itemKey: "article:test-123",
		recallScore: 0.9,
		reasons: [
			{
				type: "related_to_recent_search",
				description: "Recent search overlap",
			},
		],
		firstEligibleAt: "2026-03-17T09:00:00Z",
		nextSuggestAt: "2026-03-19T09:00:00Z",
		item: {
			itemKey: "article:test-123",
			itemType: "article",
			articleId: "test-123",
			title: "Enriched recall title",
			publishedAt: "2026-03-16T10:00:00Z",
			summaryExcerpt: "Enriched summary excerpt",
			summaryState: "ready",
			tags: ["AI", "Go", "Rust"],
			why: [{ code: "summary_completed" }],
			score: 0.81,
			url: "https://example.com/article",
		},
		...overrides,
	};
}

const defaultItem = makeCandidate().item;
if (!defaultItem) throw new Error("makeCandidate must return item");

describe("RecallCandidateCard", () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2026-03-19T12:00:00Z"));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("renders summary excerpt only when the item is ready", async () => {
		render(RecallCandidateCard as never, {
			props: {
				candidate: makeCandidate(),
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Enriched summary excerpt"))
			.toBeInTheDocument();
	});

	it("renders up to two non-empty tags", async () => {
		render(RecallCandidateCard as never, {
			props: {
				candidate: makeCandidate({
					item: {
						...defaultItem,
						tags: ["AI", " ", "Go", "Rust"],
					},
				}),
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("AI", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Go", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Rust", { exact: true }))
			.not.toBeInTheDocument();
	});

	it("uses publishedAt for age display and falls back to recent for invalid dates", async () => {
		render(RecallCandidateCard as never, {
			props: {
				candidate: makeCandidate({
					item: {
						...defaultItem,
						publishedAt: "not-a-date",
					},
				}),
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect.element(page.getByText("recent")).toBeInTheDocument();
	});

	it("falls back to itemKey when item is missing", async () => {
		render(RecallCandidateCard as never, {
			props: {
				candidate: makeCandidate({
					item: undefined,
				}),
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("article:test-123"))
			.toBeInTheDocument();
	});

	it("keeps snooze and dismiss clicks from triggering open", async () => {
		const onOpen = vi.fn();
		const onSnooze = vi.fn();
		const onDismiss = vi.fn();

		render(RecallCandidateCard as never, {
			props: {
				candidate: makeCandidate(),
				onSnooze,
				onDismiss,
				onOpen,
			},
		});

		await page.getByTitle("Snooze for 24 hours").click();
		await page.getByTitle("Dismiss").click();

		expect(onSnooze).toHaveBeenCalledWith("article:test-123");
		expect(onDismiss).toHaveBeenCalledWith("article:test-123");
		expect(onOpen).not.toHaveBeenCalled();
	});
});
