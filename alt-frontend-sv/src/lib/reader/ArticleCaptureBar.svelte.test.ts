import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	AnnotationStance,
	type ArticleAnnotationItem,
} from "#lib/connect/reading.js";

vi.mock("#lib/connect/reading-transport.js", () => ({
	getReadingTransport: () => "mock-transport",
}));

vi.mock("#lib/connect/reading.js", async (importOriginal) => {
	const actual =
		await importOriginal<typeof import("#lib/connect/reading.js")>();
	return {
		...actual,
		listArticleAnnotations: vi.fn(),
		saveArticleAnnotation: vi.fn(),
		deleteArticleAnnotation: vi.fn(),
	};
});

import {
	deleteArticleAnnotation,
	listArticleAnnotations,
	saveArticleAnnotation,
} from "#lib/connect/reading.js";
import ArticleCaptureBar from "./ArticleCaptureBar.svelte";

const ARTICLE_ID = "01938e82-7c00-7a7b-9b10-100000000011";

async function settle(ms = 60) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("ArticleCaptureBar", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(listArticleAnnotations).mockResolvedValue([]);
	});

	it("renders 4 quiet stance pills and opens inline capture on 1-tap, saving takeaway and showing Ask Augur link", async () => {
		const saved: ArticleAnnotationItem = {
			annotationId: "01938e82-7c00-7a7b-9b10-900000000001",
			articleId: ARTICLE_ID,
			stance: AnnotationStance.QUESTION,
			excerpt: "Zero-copy parsing cuts allocations by 40%.",
			note: "Does this hold under backpressure?",
			createdAt: new Date("2026-11-28T05:00:00Z"),
		};
		vi.mocked(saveArticleAnnotation).mockResolvedValueOnce(saved);
		vi.mocked(deleteArticleAnnotation).mockResolvedValueOnce(undefined);

		render(ArticleCaptureBar, {
			props: {
				articleId: ARTICLE_ID,
				articleTitle: "Zero-Copy Log Ingestion",
			},
		});
		await settle();

		expect(listArticleAnnotations).toHaveBeenCalledWith(
			"mock-transport",
			[ARTICLE_ID],
			20,
		);

		await page.getByRole("button", { name: "Question" }).click();
		await page
			.getByPlaceholder("Optional quote from the article")
			.fill("Zero-copy parsing cuts allocations by 40%.");
		await page
			.getByPlaceholder("What question did this raise?")
			.fill("Does this hold under backpressure?");
		await page.getByRole("button", { name: "Save takeaway" }).click();
		await settle();

		expect(saveArticleAnnotation).toHaveBeenCalledWith("mock-transport", {
			articleId: ARTICLE_ID,
			stance: AnnotationStance.QUESTION,
			excerpt: "Zero-copy parsing cuts allocations by 40%.",
			note: "Does this hold under backpressure?",
		});

		await expect
			.element(page.getByText("Does this hold under backpressure?"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Zero-copy parsing cuts allocations by 40%."))
			.toBeInTheDocument();

		const augurLink = page.getByRole("link", { name: "Ask Augur" });
		await expect.element(augurLink).toBeInTheDocument();
		const href = augurLink.element().getAttribute("href") ?? "";
		expect(href).toContain("/augur?");
		expect(href).toContain(`articleId=${ARTICLE_ID}`);

		// Delete takeaway
		await page.getByRole("button", { name: "Remove takeaway" }).click();
		await settle();
		expect(deleteArticleAnnotation).toHaveBeenCalledWith(
			"mock-transport",
			saved.annotationId,
		);
		await expect
			.element(page.getByText("Does this hold under backpressure?"))
			.not.toBeInTheDocument();
	});

	it("renders nothing when articleId is not a valid UUID", async () => {
		const { container } = render(ArticleCaptureBar, {
			props: {
				articleId: "not-a-uuid",
				articleTitle: "Test",
			},
		});
		await settle();

		expect(listArticleAnnotations).not.toHaveBeenCalled();
		expect(
			container.querySelector('[data-testid="article-capture-bar"]'),
		).toBeNull();
	});
});
