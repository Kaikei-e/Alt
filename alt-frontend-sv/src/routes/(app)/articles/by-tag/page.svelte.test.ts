import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagTrailArticle } from "#lib/connect/index.js";

const { pageState } = vi.hoisted(() => {
	const value: { url: URL } = {
		url: new URL("http://localhost/articles/by-tag?tag=AI"),
	};
	return {
		pageState: {
			get url() {
				return value.url;
			},
			set url(next: URL) {
				value.url = next;
			},
		},
	};
});

vi.mock("$app/state", () => ({ page: pageState }));

const { fetchArticlesByTagMock, createClientTransportMock } = vi.hoisted(
	() => ({
		fetchArticlesByTagMock: vi.fn(),
		createClientTransportMock: vi.fn(() => ({})),
	}),
);

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: createClientTransportMock,
	fetchArticlesByTag: fetchArticlesByTagMock,
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const ARTICLES: TagTrailArticle[] = [
	{
		id: "art-1",
		title: "AI Trends in 2026",
		link: "https://example.com/ai-trends",
		publishedAt: "2026-04-01T12:00:00Z",
		feedTitle: "TechBlog",
	},
	{
		id: "art-2",
		title: "Systems Engineering for LLMs",
		link: "https://example.com/systems-llm",
		publishedAt: "2026-04-02T12:00:00Z",
		feedTitle: "Systems Weekly",
	},
];

async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("/articles/by-tag page", () => {
	beforeEach(() => {
		pageState.url = new URL("http://localhost/articles/by-tag?tag=AI");
		fetchArticlesByTagMock.mockReset();
		fetchArticlesByTagMock.mockResolvedValue({
			articles: ARTICLES,
			nextCursor: null,
			hasMore: false,
		});
	});

	it("shows no-tag guidance when ?tag= query param is missing", async () => {
		pageState.url = new URL("http://localhost/articles/by-tag");
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle(150);

		await expect
			.element(page.getByText(/no tag specified/i))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).not.toHaveBeenCalled();
	});

	it("rejects unsafe javascript: and data: URLs in mobile article links", async () => {
		fetchArticlesByTagMock.mockResolvedValueOnce({
			articles: [
				{
					id: "art-unsafe-js",
					title: "Malicious JS Link",
					link: "javascript:alert(1)",
					publishedAt: "2026-04-01T12:00:00Z",
					feedTitle: "Untrusted Feed",
				},
				{
					id: "art-unsafe-data",
					title: "Malicious Data Link",
					link: "data:text/html,<script>alert(1)</script>",
					publishedAt: "2026-04-01T12:00:00Z",
					feedTitle: "Untrusted Feed",
				},
				{
					id: "art-safe",
					title: "Safe HTTPS Link",
					link: "https://example.com/safe",
					publishedAt: "2026-04-01T12:00:00Z",
					feedTitle: "Trusted Feed",
				},
			],
			nextCursor: null,
			hasMore: false,
		});

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const { container } = render(Page);
		await settle(300);

		expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
		expect(container.querySelector('a[href^="data:"]')).toBeNull();

		const safeLink = container.querySelector(
			'a[href="https://example.com/safe"]',
		);
		expect(safeLink).not.toBeNull();
		expect(safeLink?.textContent?.trim()).toBe("Safe HTTPS Link");
	});

	it("preserves loaded articles without re-fetching across 393x851 <-> 851x393 viewport rotation", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(350);

		await expect
			.element(page.getByTestId("tag-article-art-1"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("tag-article-art-2"))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).toHaveBeenCalledTimes(1);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(350);

		await expect
			.element(page.getByTestId("tag-article-art-1"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("tag-article-art-2"))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).toHaveBeenCalledTimes(1);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(350);

		await expect
			.element(page.getByTestId("tag-article-art-1"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("tag-article-art-2"))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).toHaveBeenCalledTimes(1);
	});
});
