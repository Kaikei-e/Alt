import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { RecapCard } from "$lib/connect/recap";
import RecapTopicCard from "./RecapTopicCard.svelte";

function makeCard(overrides: Partial<RecapCard> = {}): RecapCard {
	return {
		id: "card-123",
		rank: 1,
		storyId: "story-456",
		continuesCardId: undefined,
		headlineJa: "大規模推論モデルの国内展開が加速",
		whatJa:
			"最新の推論モデルが国内クラウドで提供開始された。[1]多くの開発者が即座にテストを開始した。[2]",
		whyJa: "機密データ保護と低遅延運用の両立が期待される。[1]",
		genre: "Technology",
		sources: [
			{
				n: 1,
				feedId: "feed-1",
				url: "https://example.com/topic-1/1",
				host: "example.com",
				title: "国内オープンソースAIの動向 1",
				pubDate: "2026-09-22T08:00:00Z",
			},
			{
				n: 2,
				feedId: "feed-2",
				url: "https://example.jp/articles/1-2",
				host: "example.jp",
				title: "研究機関による評価基準の策定 1",
				pubDate: "2026-09-22T09:00:00Z",
			},
		],
		createdAt: "2026-09-22T17:05:00Z",
		...overrides,
	};
}

describe("RecapTopicCard", () => {
	it("renders rank, headline, what, why, genre, and sources", async () => {
		const card = makeCard();
		render(RecapTopicCard, { props: { card } });

		await expect
			.element(page.getByText("大規模推論モデルの国内展開が加速"))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText("最新の推論モデルが国内クラウドで提供開始された。"),
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText("機密データ保護と低遅延運用の両立が期待される。"))
			.toBeInTheDocument();
		await expect.element(page.getByText("Technology")).toBeInTheDocument();
		await expect.element(page.getByText("#1")).toBeInTheDocument();

		// Sources rendered as numbered links with rel=noopener and target=_blank
		const source1 = page.getByRole("link", {
			name: /\[1\] 国内オープンソースAIの動向 1/i,
		});
		await expect.element(source1).toBeInTheDocument();
		expect(source1.element().getAttribute("target")).toBe("_blank");
		expect(source1.element().getAttribute("rel")).toContain("noopener");
		expect(source1.element().getAttribute("href")).toBe(
			"https://example.com/topic-1/1",
		);
	});

	it("omits why when whyJa is undefined or null", async () => {
		const card = makeCard({ whyJa: undefined });
		const { container } = render(RecapTopicCard, { props: { card } });

		expect(container.querySelector('[data-testid="card-why"]')).toBeNull();
	});

	it("omits genre badge when genre is undefined or null", async () => {
		const card = makeCard({ genre: undefined });
		const { container } = render(RecapTopicCard, { props: { card } });

		expect(container.querySelector('[data-testid="card-genre"]')).toBeNull();
	});

	it("renders Continues marker when continuesCardId is set", async () => {
		const card = makeCard({ continuesCardId: "card-prev-999" });
		render(RecapTopicCard, { props: { card } });

		await expect.element(page.getByText("Continues")).toBeInTheDocument();
	});

	it("omits Continues marker when continuesCardId is not set", async () => {
		const card = makeCard({ continuesCardId: undefined });
		const { container } = render(RecapTopicCard, { props: { card } });

		expect(
			container.querySelector('[data-testid="card-continues"]'),
		).toBeNull();
	});

	it("renders sources with accessible list semantics and host name", async () => {
		const card = makeCard();
		render(RecapTopicCard, { props: { card } });

		await expect.element(page.getByText("example.com")).toBeInTheDocument();
		await expect.element(page.getByText("example.jp")).toBeInTheDocument();
	});

	it("renders https URL as an external anchor with rel=noopener", async () => {
		const card = makeCard({
			sources: [
				{
					n: 1,
					feedId: "feed-1",
					url: "https://example.com/safe-link",
					host: "example.com",
					title: "Safe Article Link",
				},
			],
		});
		render(RecapTopicCard, { props: { card } });

		const link = page.getByRole("link", {
			name: /Safe Article Link/i,
		});
		await expect.element(link).toBeInTheDocument();
		expect(link.element().getAttribute("target")).toBe("_blank");
		expect(link.element().getAttribute("rel")).toContain("noopener");
		expect(link.element().getAttribute("href")).toBe(
			"https://example.com/safe-link",
		);
	});

	it("rejects javascript: URL and renders source as plain text without anchor", async () => {
		const card = makeCard({
			sources: [
				{
					n: 1,
					feedId: "feed-1",
					url: "javascript:alert(1)",
					host: "example.com",
					title: "Malicious XSS Link",
				},
			],
		});
		const { container } = render(RecapTopicCard, { props: { card } });

		// Assert no anchor exists with the title or javascript href
		expect(container.querySelector('a[href*="javascript"]')).toBeNull();
		const links = container.querySelectorAll("a");
		expect(links.length).toBe(0);

		// But the source text is still visible as plain text
		await expect
			.element(page.getByText("Malicious XSS Link"))
			.toBeInTheDocument();
	});

	it("sorts sources in ascending order of n", async () => {
		const card = makeCard({
			sources: [
				{
					n: 3,
					feedId: "feed-3",
					url: "https://example.com/3",
					host: "example.com",
					title: "Third Source",
				},
				{
					n: 1,
					feedId: "feed-1",
					url: "https://example.com/1",
					host: "example.com",
					title: "First Source",
				},
				{
					n: 2,
					feedId: "feed-2",
					url: "https://example.com/2",
					host: "example.com",
					title: "Second Source",
				},
			],
		});
		const { container } = render(RecapTopicCard, { props: { card } });

		const sourceElements = container.querySelectorAll(
			'[data-testid="card-source"]',
		);
		expect(sourceElements).toHaveLength(3);
		expect(sourceElements[0]?.textContent).toContain("[1]");
		expect(sourceElements[1]?.textContent).toContain("[2]");
		expect(sourceElements[2]?.textContent).toContain("[3]");
	});
});
