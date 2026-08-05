import { test, expect, stubURL } from "../src/fixtures.js";

/**
 * Article summary reads — the port of `26-feeds-summary.hurl`.
 *
 * `/fetch/summary/provided` reads a pre-generated Inoreader summary from the
 * database; `/fetch/summary` goes out to the summarisation path. Neither has a
 * reachable happy path against the staging fixture set (no summary rows are
 * seeded), so the Hurl file bounded both to `200 <= status < 500`.
 *
 * That band is kept — widening it would be pinning nothing — but each test now
 * also asserts the answer is *well-formed*: a 5xx is excluded, and a JSON body
 * is required. The original assertion accepted an HTML error page from a
 * crashed handler as a pass.
 */

const SUMMARY_ENDPOINTS = [
	{ name: "provided", path: "/v1/feeds/fetch/summary/provided" },
	{ name: "generated", path: "/v1/feeds/fetch/summary" },
] as const;

test.describe("summary reads", () => {
	for (const endpoint of SUMMARY_ENDPOINTS) {
		test(`${endpoint.name}: a URL with no summary answers without faulting`, async ({
			rest,
			csrf,
			seededFeeds,
		}) => {
			const feed = seededFeeds[0];
			expect(feed).toBeDefined();

			const response = await rest.post(endpoint.path, {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { feed_url: feed?.url ?? stubURL("feed-summary-fallback.xml") },
			});

			expect(response.status()).toBeGreaterThanOrEqual(200);
			expect(response.status()).toBeLessThan(500);

			// Every alt-backend REST answer is JSON — including its errors. An
			// unparseable body here means the response came from a panic handler
			// or a proxy, not from this route.
			const text = await response.text();
			expect(() => JSON.parse(text), `body was not JSON: ${text.slice(0, 500)}`).not.toThrow();
		});

		test(`${endpoint.name}: an empty feed_url is rejected`, async ({ rest, csrf }) => {
			const response = await rest.post(endpoint.path, {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { feed_url: "" },
			});
			expect(response.status()).toBeGreaterThanOrEqual(400);
			expect(response.status()).toBeLessThan(500);
		});
	}
});
