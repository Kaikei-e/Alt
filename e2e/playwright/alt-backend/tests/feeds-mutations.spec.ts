import { test, expect, stubURL } from "../src/fixtures.js";
import { expectStatusIn } from "../../_shared/http.js";

/**
 * State-mutating feed endpoints — the port of `22-feeds-read-favorite.hurl`.
 *
 * Both request shapes are pinned against the real Go structs, because both
 * were wrong in an earlier generation of this scenario and a blanket
 * `status >= 200; status < 600` assertion accepted the resulting 400s as
 * passes:
 *
 *   - `POST /v1/feeds/read` binds `ReadStatus{FeedURL}` → `feed_url`.
 *   - `POST /v1/feeds/register/favorite` binds `RssFeedLink{URL}` → `url`.
 *
 * Neither happy path is reachable against the staging fixture set, and the
 * comments below say why for each. The status bands exclude 2xx (a success
 * would mean the fixture changed and these scenarios stopped testing what
 * they claim) and exclude 4xx (which would mean the request shape regressed
 * back to binding an empty string).
 */

test.describe("POST /v1/feeds/read", () => {
	test("resolves the feed URL to a stored item", async ({ rest, csrf, seededFeeds }) => {
		// MarkFeedAsRead resolves the body's URL to a stored feed *item*. The
		// staging fixture set seeds zero article rows, so a registered feed URL
		// is never a known item. 500 is today's answer for an unresolvable URL;
		// 404 would be the correct one, so both are accepted rather than
		// pinning today's bug as the contract.
		const feed = seededFeeds[0];
		expect(feed).toBeDefined();

		const response = await rest.post("/v1/feeds/read", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { feed_url: feed?.url ?? stubURL("feed-read-fallback.xml") },
		});
		await expectStatusIn(response, [404, 500]);
	});

	test("KNOWN BUG: an empty feed_url is a 500, not a validation error", async ({ rest, csrf }) => {
		// New coverage, and it found something. `ReadStatus` carries
		// `validate:"required,url"` (schema.go:7-9), but nothing runs that
		// validator on this route: ValidationMiddleware's `validateRoute` has no
		// case for `/feeds/read`, and the handler binds and goes straight to the
		// usecase. So an empty URL is indistinguishable from an unresolvable one
		// and both come back 500.
		//
		// This is the same 400-vs-500 confusion as the malformed feed id in
		// tests/feeds-details-tags.spec.ts, and it also means the band asserted
		// above cannot distinguish "the request shape regressed to empty" from
		// "the URL did not resolve" — which is exactly what that band was meant
		// to guard. Pinning it here keeps that limitation visible instead of
		// letting the band quietly cover it.
		const response = await rest.post("/v1/feeds/read", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { feed_url: "" },
		});
		expect(response.status()).toBe(500);
	});
});

test.describe("POST /v1/feeds/register/favorite", () => {
	test("looks the URL up as a site URL, not a feed URL", async ({ rest, csrf, seededFeeds }) => {
		// The driver looks the URL up as `feeds.website_url`, which holds the
		// site URL taken from the RSS document — not the .xml feed URL that was
		// registered. So no row matches and this cannot reach its 200 against
		// the stub's synthetic documents. 4xx stays excluded so a regression
		// back to the old `feed_url` binding (which bound an empty URL and only
		// ever hit the 400 validation branch) fails the run.
		const feed = seededFeeds[0];
		expect(feed).toBeDefined();

		const response = await rest.post("/v1/feeds/register/favorite", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: feed?.url ?? stubURL("feed-fav-fallback.xml") },
		});
		await expectStatusIn(response, [404, 500]);
	});

	test("an empty url is a validation error", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/feeds/register/favorite", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: "" },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});
