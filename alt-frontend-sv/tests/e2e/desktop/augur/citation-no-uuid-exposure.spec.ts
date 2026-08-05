import { expect, test } from "../../fixtures/pomFixtures";

/**
 * The internal alt-db UUID identifies an article on the wire (refId) and in
 * the `/articles/<id>` route, but it must never appear as visible text in the
 * citation rail. ADR-926 fixed the link-target bug; this spec enforces the
 * remaining visible-text invariant: if the backend regresses and emits a Title
 * that is just a UUID, the FE downgrades it to the URL's domain or to
 * "Untitled source" instead of letting the raw UUID render.
 */
test.describe("Augur Citation UUID Exposure", () => {
	const conversationId = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
	const articleRefId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
	const isoTimestamp = "2023-11-14T22:13:20Z";

	test("UUID-only Title falls back to URL domain, never the raw UUID", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			(route) =>
				route.fulfill({
					status: 200,
					contentType: "application/json",
					body: JSON.stringify({
						id: conversationId,
						title: "UUID exposure regression check",
						createdAt: isoTimestamp,
						messages: [
							{
								role: "assistant",
								content: "Backend regressed; Title carries a bare UUID.",
								createdAt: isoTimestamp,
								citations: [
									{
										url: "https://news.example.test/posts/hello",
										title: articleRefId,
										publishedAt: isoTimestamp,
										kind: "CITATION_KIND_ARTICLE",
										refId: articleRefId,
									},
								],
								relatedCitations: [],
							},
						],
					}),
				}),
		);

		await page.goto(`/augur/${conversationId}`);

		// Wait for the rail to actually render before checking its text —
		// otherwise a premature innerText() read on an empty/absent rail
		// would trivially "pass" without ever exercising the invariant.
		const citationRail = page.locator(".citation-rail");
		await expect(citationRail).toBeVisible();

		// The visible text on every citation row must not contain the UUID.
		await expect(citationRail).not.toContainText(articleRefId);

		// The fallback for a UUID-only Title is the URL's domain.
		const titleLink = page
			.locator(".citation-rail a.item-title")
			.filter({ hasText: "news.example.test" });
		await expect(titleLink).toHaveAttribute(
			"href",
			`/articles/${articleRefId}`,
		);
	});

	test("Empty Title with no URL falls back to 'Untitled source'", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			(route) =>
				route.fulfill({
					status: 200,
					contentType: "application/json",
					body: JSON.stringify({
						id: conversationId,
						title: "Empty title fallback",
						createdAt: isoTimestamp,
						messages: [
							{
								role: "assistant",
								content: "No title and no URL.",
								createdAt: isoTimestamp,
								citations: [
									{
										url: "",
										title: "",
										publishedAt: "",
										kind: "CITATION_KIND_ARTICLE",
										refId: articleRefId,
									},
								],
								relatedCitations: [],
							},
						],
					}),
				}),
		);

		await page.goto(`/augur/${conversationId}`);

		const citationRail = page.locator(".citation-rail");
		await expect(citationRail).toBeVisible();
		await expect(citationRail).not.toContainText(articleRefId);
		await expect(
			page.locator(".citation-rail a.item-title").filter({
				hasText: "Untitled source",
			}),
		).toBeVisible();
	});
});
