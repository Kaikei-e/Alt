import { expect, test } from "@playwright/test";

/**
 * Citation links inside an Augur conversation must route to the canonical
 * surface for the cited artefact, never back into the `/augur/<id>` namespace.
 *
 * A `SUMMARY` or `ARTICLE` citation carries a `ref_id` that is an alt-db
 * `articles.id` / `summary_versions.summary_version_id` UUID; clicking it must
 * land the user on the article detail page. A `WEB` citation carries a full
 * URL in `url`; clicking it opens externally. Anything unrecognised (legacy
 * payload with a bare UUID stuffed into `url`, or `CITATION_KIND_UNSPECIFIED`)
 * must render without a link so the relative-URL bug — `<a href="<uuid>">`
 * resolving to `/augur/<uuid>` — cannot recur.
 */
test.describe("Augur Citation Link Routing", () => {
	const conversationId = "11111111-1111-4111-8111-111111111111";
	const summaryRefId = "22222222-2222-4222-8222-222222222222";
	const articleRefId = "33333333-3333-4333-8333-333333333333";
	const externalUrl = "https://example.test/posts/google-health-fitbit";
	const legacyBareUuid = "44444444-4444-4444-8444-444444444444";

	// protobuf-es uses proto3 JSON wire format: Timestamp is an RFC 3339 string,
	// not a {seconds, nanos} object. Enums accept the full proto name (e.g.
	// CITATION_KIND_SUMMARY) when sent as JSON.
	const isoTimestamp = "2023-11-14T22:13:20Z";
	const mockGetConversation = (route: import("@playwright/test").Route) =>
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
						content:
							"From your Knowledge Loop: test answer with mixed citations.",
						createdAt: isoTimestamp,
						citations: [
							{
								url: "",
								title: "summary",
								publishedAt: "",
								kind: "CITATION_KIND_SUMMARY",
								refId: summaryRefId,
							},
							{
								url: "",
								title: "article",
								publishedAt: "",
								kind: "CITATION_KIND_ARTICLE",
								refId: articleRefId,
							},
							{
								url: externalUrl,
								title: "Reference",
								publishedAt: "",
								kind: "CITATION_KIND_WEB",
								refId: "",
							},
							{
								url: legacyBareUuid,
								title: "legacy",
								publishedAt: "",
							},
						],
					},
				],
			}),
		});

	test("summary citation links to /articles/<refId>, not /augur/<refId>", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			mockGetConversation,
		);

		await page.goto(`/augur/${conversationId}`);

		const summaryLink = page
			.locator(".citation-rail a.item-title")
			.filter({ hasText: /^summary$/ })
			.first();
		await expect(summaryLink).toHaveAttribute(
			"href",
			`/articles/${summaryRefId}`,
		);
	});

	test("article citation links to /articles/<refId>", async ({ page }) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			mockGetConversation,
		);

		await page.goto(`/augur/${conversationId}`);

		const articleLink = page
			.locator(".citation-rail a.item-title")
			.filter({ hasText: /^article$/ })
			.first();
		await expect(articleLink).toHaveAttribute(
			"href",
			`/articles/${articleRefId}`,
		);
	});

	test("web citation links to the external URL", async ({ page }) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			mockGetConversation,
		);

		await page.goto(`/augur/${conversationId}`);

		const webLink = page
			.locator(".citation-rail a.item-title")
			.filter({ hasText: /^Reference$/ })
			.first();
		await expect(webLink).toHaveAttribute("href", externalUrl);
	});

	test("legacy bare-UUID citation renders without an anchor", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.augur.v2.AugurService/GetConversation",
			mockGetConversation,
		);

		await page.goto(`/augur/${conversationId}`);

		const legacyTitle = page
			.locator(".citation-rail .item-title")
			.filter({ hasText: /^legacy$/ })
			.first();
		await expect(legacyTitle).toBeVisible();
		await expect(legacyTitle).toHaveJSProperty("tagName", "SPAN");
	});
});
