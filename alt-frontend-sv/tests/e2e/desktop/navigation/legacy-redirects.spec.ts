import { expect, test } from "../../fixtures/pomFixtures";

/**
 * The app used to expose separate `/desktop/*` and `/mobile/*` route trees and
 * now serves one responsive tree. `hooks.server.ts` keeps the old paths alive
 * by 303-ing them onto the new ones, which is the only thing standing between a
 * bookmark or an external link made before the migration and a 404.
 *
 * That table is pure server logic with 30-odd entries and had exactly one
 * incidental test (a spec that happened to `goto("./desktop/feeds/search")`).
 * Every entry is asserted here, at the HTTP layer rather than through a
 * rendered page, so a failure names the broken mapping instead of some
 * downstream component.
 *
 * Kept deliberately parallel to `RESPONSIVE_REDIRECTS` in
 * `src/lib/server/redirect-resolver.ts`: a redirect added there without a row
 * here is a redirect nothing checks.
 */

/** [legacy path, responsive target] — mirrors RESPONSIVE_REDIRECTS. */
const REDIRECTS: ReadonlyArray<readonly [string, string]> = [
	["/desktop/feeds", "/feeds"],
	["/mobile/feeds", "/feeds"],
	["/desktop/augur", "/augur"],
	["/mobile/retrieve/ask-augur", "/augur"],
	["/desktop/recap/morning-letter", "/recap/morning-letter"],
	["/mobile/recap/morning-letter", "/recap/morning-letter"],
	["/desktop/feeds/tag-trail", "/feeds/tag-trail"],
	["/mobile/feeds/tag-trail", "/feeds/tag-trail"],
	["/desktop/feeds/tag-verse", "/feeds/tag-verse"],
	["/desktop/feeds/favorites", "/feeds/favorites"],
	["/desktop/feeds/search", "/feeds/search"],
	["/mobile/feeds/search", "/feeds/search"],
	["/desktop/feeds/viewed", "/feeds/viewed"],
	["/mobile/feeds/viewed", "/feeds/viewed"],
	["/desktop/recap/evening-pulse", "/recap/evening-pulse"],
	["/mobile/recap/evening-pulse", "/recap/evening-pulse"],
	["/desktop/recap", "/recap"],
	["/mobile/recap/3days", "/recap"],
	["/mobile/recap/7days", "/recap?window=7"],
	["/desktop/recap/job-status", "/recap/job-status"],
	["/mobile/recap/job-status", "/recap/job-status"],
	["/desktop/settings/feeds", "/settings/feeds"],
	["/mobile/feeds/manage", "/settings/feeds"],
	["/desktop/stats", "/stats"],
	["/mobile/feeds/stats", "/stats"],
	["/mobile/feeds/swipe", "/feeds/swipe"],
	["/desktop", "/dashboard"],
];

test.describe("legacy route redirects", () => {
	for (const [legacy, target] of REDIRECTS) {
		test(`${legacy} redirects to ${target}`, async ({ page }) => {
			const response = await page.request.get(legacy, { maxRedirects: 0 });

			// 303 rather than 301: iOS Safari pins a 301 in its cache, which would
			// freeze an old bookmark onto today's mapping forever.
			expect(response.status()).toBe(303);
			expect(response.headers().location).toBe(target);
		});
	}

	test("preserves the query string across a redirect", async ({ page }) => {
		const response = await page.request.get("/desktop/feeds?sort=title_asc", {
			maxRedirects: 0,
		});

		expect(response.status()).toBe(303);
		expect(response.headers().location).toBe("/feeds?sort=title_asc");
	});

	test("merges the query string when the target already has one", async ({
		page,
	}) => {
		// `/mobile/recap/7days` maps to `/recap?window=7`, so an incoming query
		// has to join with `&` — appending a second `?` would silently swallow it.
		const response = await page.request.get("/mobile/recap/7days?genre=tech", {
			maxRedirects: 0,
		});

		expect(response.status()).toBe(303);
		expect(response.headers().location).toBe("/recap?window=7&genre=tech");
	});

	test("leaves a responsive route alone", async ({ page }) => {
		// The resolver must not match paths outside its table, or the new routes
		// would redirect to themselves.
		const response = await page.request.get("/feeds", { maxRedirects: 0 });

		expect(response.status()).toBe(200);
	});
});
