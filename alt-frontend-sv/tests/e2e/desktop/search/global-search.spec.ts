import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

const SEARCH_EVERYTHING_PATH =
	"**/api/v2/alt.search.v2.GlobalSearchService/SearchEverything";

const MOCK_GLOBAL_SEARCH_RESPONSE = {
	query: "rust",
	articleSection: {
		hits: [
			{
				id: "art-rust-1",
				title: "Async Rust in Production",
				snippet: "Practical patterns for Tokio and io_uring.",
				link: "https://example.com/async-rust",
				tags: ["rust", "tokio"],
				matchedFields: ["title", "content"],
			},
		],
		estimatedTotal: "1",
		hasMore: false,
	},
	recapSection: {
		hits: [
			{
				id: "recap-rust-1",
				jobId: "job-rust-1",
				genre: "Systems Programming",
				summary: "Weekly roundup of Rust compiler and runtime improvements.",
				topTerms: ["rust", "compiler", "async"],
				tags: ["rust"],
				windowDays: 7,
				executedAt: "2026-03-20T12:00:00Z",
			},
		],
		estimatedTotal: "1",
		hasMore: false,
	},
	tagSection: {
		hits: [
			{
				tagName: "rust",
				articleCount: 24,
			},
		],
		total: "1",
	},
	degradedSections: [],
	searchedAt: "2026-03-25T12:00:00Z",
};

test.describe("Desktop Global Search (/search)", () => {
	test("executes search from ?q= query parameter and renders Article, Recap, and Tag sections", async ({
		page,
	}) => {
		let capturedBody: Record<string, unknown> | null = null;
		await page.route(SEARCH_EVERYTHING_PATH, async (route) => {
			capturedBody = route.request().postDataJSON();
			await fulfillJson(route, MOCK_GLOBAL_SEARCH_RESPONSE);
		});

		await page.goto("./search?q=rust");

		await expect(
			page.getByRole("heading", { name: "Reference Desk" }),
		).toBeVisible();
		await expect
			.poll(() => capturedBody, { timeout: 10000 })
			.toMatchObject({ query: "rust" });

		// Article section
		const articlesSection = page.locator(
			'[data-role="reference-articles-section"]',
		);
		await expect(articlesSection).toBeVisible();
		await expect(
			articlesSection.getByText("Async Rust in Production"),
		).toBeVisible();

		// Recap section
		const recapsSection = page.locator(
			'[data-role="reference-recaps-section"]',
		);
		await expect(recapsSection).toBeVisible();
		await expect(recapsSection.getByText("Systems Programming")).toBeVisible();

		// Tag section
		const tagsSection = page.locator('[data-role="reference-tags-section"]');
		await expect(tagsSection).toBeVisible();
		await expect(tagsSection.getByText("rust")).toBeVisible();
		await expect(tagsSection.getByText("(24)")).toBeVisible();
	});

	test("renders degraded section warning banner when degradedSections is non-empty", async ({
		page,
	}) => {
		await page.route(SEARCH_EVERYTHING_PATH, (route) =>
			fulfillJson(route, {
				...MOCK_GLOBAL_SEARCH_RESPONSE,
				recapSection: null,
				degradedSections: ["recaps"],
			}),
		);

		await page.goto("./search?q=rust");

		const banner = page.getByRole("status");
		await expect(banner).toBeVisible();
		await expect(banner).toContainText("Some sections are unavailable: recaps");
		await expect(
			page.locator('[data-role="reference-articles-section"]'),
		).toBeVisible();
		await expect(
			page.locator('[data-role="reference-recaps-section"]'),
		).not.toBeVisible();
	});
});
