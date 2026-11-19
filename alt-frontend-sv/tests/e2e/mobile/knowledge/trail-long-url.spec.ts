import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

const LONG_URL =
	"https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/77bc4efe8fc0d630a7f6f1c9dsc09454.jpg?auto=webp&fit=crop&height=675&width=1200";

const TRAIL_WITH_LONG_URL = {
	footprints: [],
	nextCursor: "",
	hasMore: false,
	generatedAt: "2026-07-18T00:00:00Z",
	branches: [
		{
			branchKey: "continuation:u1:article:long-url",
			anchorItemKey: "article:long-url",
			relationKind: "continuation",
			why: LONG_URL,
			evidenceRefs: [{ refId: "src", label: LONG_URL, kind: "source" }],
			confidence: "high",
			targetItemKey: "article:target-url",
			targetTitle: LONG_URL,
		},
	],
	episodes: [
		{
			episodeKey: "episode:article:long-url",
			wear: "worn",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:long-url",
					verb: "read",
					itemKey: "article:long-url",
					title: LONG_URL,
					excerpt: LONG_URL,
					tags: [LONG_URL],
					note: LONG_URL,
					occurredAt: "2026-07-07T22:20:00Z",
					firstOccurredAt: "2026-06-27T18:37:00Z",
					contactCount: 2,
					wear: "worn",
				},
			],
		},
	],
};

// iPhone SE / the viewport the mobile trail must not scroll past.
test.use({ viewport: { width: 375, height: 667 } });

test.describe("Knowledge Trail mobile long URL", () => {
	test("wraps an unbroken source URL without horizontal page scroll", async ({
		page,
	}) => {
		await page.route("**/api/v2/**", (route) => fulfillJson(route, {}));
		await page.route(
			"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
			(route) => fulfillJson(route, TRAIL_WITH_LONG_URL),
		);

		await page.goto("./knowledge/trail");
		const link = page.getByTestId("episode-link");
		await expect(link).toBeVisible({ timeout: 15000 });
		await expect(link).toContainText(LONG_URL);

		const fit = await page.evaluate(() => ({
			scrollWidth: document.documentElement.scrollWidth,
			clientWidth: document.documentElement.clientWidth,
		}));
		expect(fit.scrollWidth).toBeLessThanOrEqual(fit.clientWidth);

		await page.screenshot({
			path: "test-results/knowledge-trail-mobile-long-url-after.png",
			fullPage: true,
		});
	});
});
