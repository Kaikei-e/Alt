import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * The reader masthead on an id-only entry.
 *
 * Two entry points reach /articles/<id> with no `?title=` handoff: the
 * summary-ready push notification, whose navigate target is the bare
 * `/articles/<id>`, and any Knowledge Home card whose projection row carries a
 * blank title. Both used to render the URL host as the headline, so a Guardian
 * feature appeared as "www.theguardian.com". The title has to be resolved from
 * the article id server-side, the same way the source URL already is.
 */

const ARTICLE_ID = "8da59f57-1906-459a-9062-e8e59815c7d0";
const SOURCE_URL =
	"https://www.theguardian.com/lifeandstyle/2026/aug/11/job-that-changed-me";
const SOURCE_HOST = "www.theguardian.com";
const RESOLVED_TITLE =
	"A job that changed me: working at a toy store was my teen feminist awakening";

const GET_ARTICLE_SOURCE_URL =
	"**/api/v2/alt.articles.v2.ArticleService/GetArticleSourceURL";

test.describe("Article masthead on an id-only entry", () => {
	test.use({ viewport: { width: 375, height: 812 } });

	test.beforeEach(async ({ page }) => {
		// Catch-all first: Playwright matches the most recently registered route,
		// so the specific handler below wins while every other RPC the page fires
		// on mount answers empty-valid instead of 401-ing the page into a redirect.
		await page.route("**/api/v2/**", (route) => fulfillJson(route, {}));
		await page.route(GET_ARTICLE_SOURCE_URL, (route) =>
			fulfillJson(route, { sourceUrl: SOURCE_URL, title: RESOLVED_TITLE }),
		);
	});

	test("renders the resolved headline when no ?title= is passed", async ({
		page,
	}) => {
		await page.goto(`./articles/${ARTICLE_ID}`);

		await expect(page.getByRole("heading", { level: 1 })).toHaveText(
			RESOLVED_TITLE,
			{ timeout: 15000 },
		);
	});

	test("never falls back to the bare source host", async ({ page }) => {
		await page.goto(`./articles/${ARTICLE_ID}`);

		const heading = page.getByRole("heading", { level: 1 });
		await expect(heading).toBeVisible({ timeout: 15000 });
		await expect(heading).not.toHaveText(SOURCE_HOST);
	});

	test("prefers an explicit ?title= handoff over the resolved one", async ({
		page,
	}) => {
		// The home / search entry points already pass a title; resolving must not
		// overwrite what the caller supplied.
		const handoff = "Title from the calling surface";
		await page.goto(
			`./articles/${ARTICLE_ID}?title=${encodeURIComponent(handoff)}`,
		);

		await expect(page.getByRole("heading", { level: 1 })).toHaveText(handoff, {
			timeout: 15000,
		});
	});
});
