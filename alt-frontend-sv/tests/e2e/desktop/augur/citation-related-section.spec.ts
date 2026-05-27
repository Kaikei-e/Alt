import { expect, test } from "@playwright/test";

/**
 * GetConversation now returns both `citations` (direct, grounded by the LLM)
 * and `related_citations` (an inline-projected snapshot of articles near the
 * direct set). The CitationRail must surface them as sibling sections so the
 * reader can pivot between "what this answer cites" and "what to read next".
 *
 * Empty `related_citations` MUST collapse the Related section; legacy
 * conversations persisted before the column existed will arrive with an empty
 * array and the UI must render exactly as it did before this change.
 */
test.describe("Augur Related Citation Section", () => {
	const conversationId = "55555555-5555-4555-8555-555555555555";
	const directRefIdA = "66666666-6666-4666-8666-666666666666";
	const directRefIdB = "77777777-7777-4777-8777-777777777777";
	const relatedRefIdX = "88888888-8888-4888-8888-888888888888";
	const relatedRefIdY = "99999999-9999-4999-8999-999999999999";
	const relatedRefIdZ = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
	const isoTimestamp = "2023-11-14T22:13:20Z";

	const mockGetConversation =
		(related: object[]) => (route: import("@playwright/test").Route) =>
			route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({
					id: conversationId,
					title: "Test Conversation",
					createdAt: isoTimestamp,
					messages: [
						{
							role: "assistant",
							content: "Answer grounded in two direct citations.",
							createdAt: isoTimestamp,
							citations: [
								{
									url: "",
									title: "Direct Article A",
									publishedAt: isoTimestamp,
									kind: "CITATION_KIND_ARTICLE",
									refId: directRefIdA,
								},
								{
									url: "",
									title: "Direct Article B",
									publishedAt: isoTimestamp,
									kind: "CITATION_KIND_ARTICLE",
									refId: directRefIdB,
								},
							],
							relatedCitations: related,
						},
					],
				}),
			});

	test("Direct + Related sections render as siblings", async ({ page }) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			mockGetConversation([
				{
					url: "",
					title: "Neighbor X",
					publishedAt: isoTimestamp,
					kind: "CITATION_KIND_ARTICLE",
					refId: relatedRefIdX,
				},
				{
					url: "",
					title: "Neighbor Y",
					publishedAt: isoTimestamp,
					kind: "CITATION_KIND_ARTICLE",
					refId: relatedRefIdY,
				},
				{
					url: "",
					title: "Neighbor Z",
					publishedAt: isoTimestamp,
					kind: "CITATION_KIND_ARTICLE",
					refId: relatedRefIdZ,
				},
			]),
		);

		await page.goto(`/augur/${conversationId}`);

		const citationsHeading = page.locator(
			".citation-rail #rail-citations-heading",
		);
		await expect(citationsHeading).toBeVisible();

		const relatedHeading = page.locator(".citation-rail #rail-related-heading");
		await expect(relatedHeading).toBeVisible();

		// Both sections route articles through /articles/<refId>.
		await expect(
			page
				.locator(".citation-rail .rail-list a.item-title")
				.filter({ hasText: "Direct Article A" }),
		).toHaveAttribute("href", `/articles/${directRefIdA}`);
		await expect(
			page
				.locator(".citation-rail .rail-list-related a.item-title")
				.filter({ hasText: "Neighbor X" }),
		).toHaveAttribute("href", `/articles/${relatedRefIdX}`);

		const relatedItems = page.locator(".citation-rail .rail-list-related li");
		await expect(relatedItems).toHaveCount(3);
	});

	test("Empty related_citations collapses the Related section", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			mockGetConversation([]),
		);

		await page.goto(`/augur/${conversationId}`);

		await expect(
			page.locator(".citation-rail #rail-citations-heading"),
		).toBeVisible();
		await expect(
			page.locator(".citation-rail #rail-related-heading"),
		).toHaveCount(0);
	});
});
