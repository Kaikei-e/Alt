import {
	buildResolveOgImagesResponse,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * The swipe card's og:image, end to end.
 *
 * The mobile swipe surface was the last one still running its own image
 * pipeline: a bare `<img>` whose `onerror` latched for the life of the card,
 * fed by whatever URL happened to be at hand — including
 * `FetchArticleContent.og_image_url`, which is the publisher's own URL. This
 * spec pins the two properties the migration to `createProxyImage` buys:
 *
 *  - the card fills in for a feed whose RSS carried no og:image, by asking
 *    `ResolveOgImages` on **feeds.id** once the reader is looking at it;
 *  - nothing but the HMAC-signed `/v1/images/proxy/...` path ever reaches the
 *    DOM, even when a raw publisher URL is on offer.
 *
 * Mirrors `tests/e2e/desktop/feeds/visual-preview-ondemand-og-resolve.spec.ts`
 * for the desktop grid.
 *
 * Unlike that spec, the feed list here cannot be stubbed with `page.route`:
 * this route sets `ssr = true` so the LCP image is in the initial HTML, and the
 * first cards are loaded by `+page.server.ts` from the mock backend before the
 * browser makes any request at all. The fixtures below therefore come from
 * `tests/e2e/infra/data/feeds.ts`, where `id` ("feed-1") and `feedId`
 * ("feeds-row-1") are deliberately different strings — a fixture where they
 * agreed would keep passing if the card regressed to sending `id`.
 */

// 1x1 transparent PNG.
const PNG_1x1 = Buffer.from(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	"base64",
);

/** feeds.id of the first mock feed. The render key is "feed-1". */
const FIRST_FEED_ID = "feeds-row-1";
const FIRST_RENDER_KEY = "feed-1";

/** The publisher's own image URL — offered by the article RPC, never rendered. */
const RAW_PUBLISHER_IMAGE = "https://cdn.example.com/hero.jpg";

/**
 * The article body arrives carrying the publisher's raw og:image and no signed
 * one: the exact shape that used to reach the card's `<img src>` through the
 * prefetcher's cache.
 */
async function stubArticleAndProxy(page: import("@playwright/test").Page) {
	await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
		fulfillJson(route, {
			url: "https://example.com/ai-trends",
			content: "<p>Full article body.</p>",
			articleId: "article-1",
			ogImageUrl: RAW_PUBLISHER_IMAGE,
			ogImageProxyUrl: "",
		}),
	);
	await page.route("**/v1/images/proxy/**", (route) =>
		route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
	);
}

test.describe("Visual Preview swipe — og:image", () => {
	test("a card with no proxy URL resolves one on feeds.id and renders it", async ({
		page,
	}) => {
		await stubArticleAndProxy(page);

		const resolveRequests: string[][] = [];
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			const ids = body?.feedIds ?? [];
			resolveRequests.push(ids);
			await fulfillJson(route, buildResolveOgImagesResponse({ resolved: ids }));
		});

		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card");
		await expect(card).toBeVisible({ timeout: 15000 });

		// The whole point: the image arrives even though neither the feed row
		// nor the article carried a signed URL.
		await expect(card.getByTestId("thumbnail-image")).toBeVisible({
			timeout: 15000,
		});
		await expect(card.getByTestId("thumbnail-fallback")).toHaveCount(0);

		const asked = resolveRequests.flat();
		expect(asked).toContain(FIRST_FEED_ID);
		// A client that regressed to sending the render key would ask for
		// "feed-1" and be told nothing has an image.
		expect(asked).not.toContain(FIRST_RENDER_KEY);
	});

	test("only the signed proxy path reaches the DOM, never the publisher's URL", async ({
		page,
	}) => {
		await stubArticleAndProxy(page);
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			await fulfillJson(
				route,
				buildResolveOgImagesResponse({ resolved: body?.feedIds ?? [] }),
			);
		});

		// Anything the page asks the publisher's CDN for is the defect: the raw
		// URL is on offer from FetchArticleContent, and the point is that it
		// goes nowhere near the reader's browser.
		const rawImageRequests: string[] = [];
		await page.route("https://cdn.example.com/**", (route) => {
			rawImageRequests.push(route.request().url());
			return route.abort();
		});

		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card");
		await expect(card).toBeVisible({ timeout: 15000 });

		const thumbnail = card.getByTestId("thumbnail-image");
		await expect(thumbnail).toBeVisible({ timeout: 15000 });
		await expect(thumbnail).toHaveAttribute("src", /^\/v1\/images\/proxy\//, {
			timeout: 15000,
		});

		expect(rawImageRequests).toEqual([]);
	});

	test("a feed whose image cannot be resolved settles on the fallback", async ({
		page,
	}) => {
		await stubArticleAndProxy(page);

		let resolveCalls = 0;
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			resolveCalls += 1;
			// The origin was asked and refused — a retry bar of zero, which is
			// "settled for this retention window", not "ask again".
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			await fulfillJson(
				route,
				buildResolveOgImagesResponse({ settled: body?.feedIds ?? [] }),
			);
		});

		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card");
		await expect(card).toBeVisible({ timeout: 15000 });

		// "No preview" rather than a shimmer that runs for the rest of the
		// session — the bounded ladder gives up and says so.
		await expect(card.getByTestId("thumbnail-fallback")).toBeVisible({
			timeout: 15000,
		});
		await expect(card.getByTestId("thumbnail-image")).toHaveCount(0);
		expect(resolveCalls).toBeGreaterThan(0);
	});
});
