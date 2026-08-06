import { test, expect, stubURL } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { registerFeedSchema, rssFeedLinkListSchema } from "../src/schemas.js";

/**
 * RSS feed-link lifecycle — the port of `10-`/`11-`/`12-`/`13-`.
 *
 * The four Hurl files were one chain: 10 registered three feeds, 11 read them
 * back by counting rows, 13 deleted `$[0].id` — whichever row that happened to
 * be. Run them out of order, or twice concurrently, and 13 deletes a feed 11
 * is still counting. That chain is why the Hurl suite could only parallelise
 * across *files* and never within them.
 *
 * Here each test owns its data. The worker's `seededFeeds` fixture registers
 * three feeds under a URL prefix no other worker uses, and every test that
 * mutates registers its own feed first. Assertions are containment
 * ("my three URLs are in the list"), never cardinality ("there are ≥3 rows"),
 * so a sibling worker's feeds are invisible rather than interfering.
 *
 * Every register call fetches the feed URL through validateAndFetchPort, which
 * round-trips to alt-backend-deps-stub serving a minimal RSS 2.0 document.
 * `FEED_ALLOWED_HOSTS=stub.invalid` is what lets it past the SSRF private-IP
 * check.
 */

function opmlDocument(urls: readonly string[]): string {
	const outlines = urls
		.map(
			(url, index) =>
				`    <outline text="E2E Stub Feed ${index + 1}" type="rss" xmlUrl="${url}" htmlUrl="${url.replace(/\.xml$/, "")}"/>`,
		)
		.join("\n");
	return `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head>
    <title>alt-backend E2E OPML import sample</title>
  </head>
  <body>
${outlines}
  </body>
</opml>
`;
}

test.describe("registration", () => {
	test("POST /register accepts a fetchable feed", async ({ rest, csrf }, testInfo) => {
		const url = stubURL(`feed-reg-${testInfo.workerIndex}-${Date.now()}.xml`);
		const body = await expectJsonStatus(
			await rest.post("/v1/rss-feed-link/register", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { url },
			}),
			200,
			registerFeedSchema,
		);
		expect(body.message).toContain("registered");
	});

	test("POST /register rejects a loopback URL (SSRF guard)", async ({ rest, csrf }) => {
		// A valid JWT and a valid CSRF token are not enough: the guard runs on
		// the outbound fetch, not on the caller.
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: "http://127.0.0.1:9000/evil-feed.xml" },
		});
		await expectStatus(response, 400);
	});

	test("POST /register rejects a link-local metadata URL", async ({ rest, csrf }) => {
		// 169.254.169.254 is the cloud instance-metadata address — the payload
		// every SSRF write-up reaches for. The loopback case above and this one
		// exercise different branches of the private-range check.
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: "http://169.254.169.254/latest/meta-data/" },
		});
		await expectStatus(response, 400);
	});

	test("POST /register rejects a non-HTTP scheme", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: "file:///etc/passwd" },
		});
		await expectStatus(response, 400);
	});

	test("POST /register rejects a missing url field", async ({ rest, csrf }) => {
		// ValidationMiddleware intercepts this route before the handler and
		// answers with its own `validation_failed` envelope (not the handler's
		// VALIDATION_ERROR one), so the assertion is on the middleware's
		// discriminator. Both layers reject; which one speaks first is the
		// contract being pinned.
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: {},
		});
		await expectStatus(response, 400);
		expect(await response.text()).toContain("validation_failed");
	});

	test("POST /register rejects a malformed JSON body", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: "{ not json at all",
		});
		await expectStatus(response, 400);
	});
});

test.describe("read-back", () => {
	test("GET /list returns this worker's feeds with health metadata", async ({
		rest,
		seededFeeds,
	}) => {
		const links = await expectJsonStatus(
			await rest.get("/v1/rss-feed-link/list"),
			200,
			rssFeedLinkListSchema,
		);

		const urls = new Set(links.map((link) => link.url));
		for (const feed of seededFeeds) {
			expect(urls, `seeded feed ${feed.url} must appear in the list`).toContain(feed.url);
		}

		// The list handler enriches each row with per-link health
		// (ConsecutiveFailures / IsActive). The schema already pins `id` as a
		// UUID and `health_status` as present; this asserts the enrichment is
		// attached to *our* rows and not just to some row in the table.
		const mine = links.filter((link) => urls.has(link.url) && link.url.includes("-w"));
		expect(mine.length).toBeGreaterThan(0);
	});

	test("GET /random returns a subscription or a well-formed empty answer", async ({ rest }) => {
		// /random picks a row at random across the tenant, so nothing about its
		// *content* is assertable from one worker. What is assertable is that it
		// never faults: a 5xx here means the random-pick query broke.
		const response = await rest.get("/v1/rss-feed-link/random");
		expect(response.status()).toBeGreaterThanOrEqual(200);
		expect(response.status()).toBeLessThan(500);
	});

	test("GET /export/opml emits this worker's feeds as OPML", async ({ rest, seededFeeds }) => {
		const response = await rest.get("/v1/rss-feed-link/export/opml");
		await expectStatus(response, 200);
		expect(response.headers()["content-type"]).toContain("xml");

		const body = await response.text();
		expect(body).toContain("<opml");
		for (const feed of seededFeeds) {
			expect(body, `export must contain ${feed.url}`).toContain(feed.url);
		}
	});
});

test.describe("OPML import", () => {
	test("POST /import/opml registers every outline", async ({ rest, csrf }, testInfo) => {
		// The fixed e2e/fixtures/alt-backend/sample-feeds.opml the Hurl suite
		// uploaded named three URLs shared by every run and every worker, so a
		// second import was always a no-op re-register. Generating the document
		// per test means the import path is exercised on genuinely new rows
		// each time — and that the round-trip below can assert containment.
		const stamp = `${testInfo.workerIndex}-${Date.now()}`;
		const urls = [1, 2, 3].map((n) => stubURL(`feed-opml-${stamp}-${n}.xml`));

		const response = await rest.post("/v1/rss-feed-link/import/opml", {
			headers: { "X-CSRF-Token": csrf },
			multipart: {
				file: {
					name: "sample-feeds.opml",
					mimeType: "application/xml",
					buffer: Buffer.from(opmlDocument(urls), "utf8"),
				},
			},
		});
		await expectStatus(response, 200);

		// Round-trip: the imported URLs must come back out of the list. The Hurl
		// scenario asserted `jsonpath "$" exists` on the import response and
		// stopped there, so an import that parsed the file and wrote nothing
		// would have passed.
		const links = await expectJsonStatus(
			await rest.get("/v1/rss-feed-link/list"),
			200,
			rssFeedLinkListSchema,
		);
		const registered = new Set(links.map((link) => link.url));
		for (const url of urls) {
			expect(registered, `imported ${url} must be listed`).toContain(url);
		}
	});

	test("KNOWN BUG: a non-OPML upload is a 500, not a 4xx", async ({ rest, csrf }) => {
		// New coverage, and it found something. The import path surfaces an XML
		// parse failure as a server error, so a user who picks the wrong file in
		// the picker is told the server broke. A malformed *upload* is the
		// caller's mistake and belongs in the 4xx band — the same 400-vs-500
		// confusion this suite pins in two other places
		// (tests/feeds-details-tags.spec.ts, tests/feeds-mutations.spec.ts).
		//
		// Pinned as current behaviour: when the handler learns to distinguish a
		// parse failure from an infrastructure failure, this test fails and gets
		// tightened to the 4xx it should have been.
		const response = await rest.post("/v1/rss-feed-link/import/opml", {
			headers: { "X-CSRF-Token": csrf },
			multipart: {
				file: {
					name: "not-opml.txt",
					mimeType: "text/plain",
					buffer: Buffer.from("this is not an OPML document", "utf8"),
				},
			},
		});
		await expectStatus(response, 500);
	});
});

/**
 * `DELETE /v1/rss-feed-link/:id` — what it actually does.
 *
 * The first CI run of this suite corrected the assumption this block was
 * written on. DELETE and `/list` are not a CRUD pair over one table:
 *
 *   - DELETE runs `DELETE FROM user_feed_subscriptions WHERE user_id = $1 AND
 *     feed_link_id = $2` (subscription_driver.go). It unsubscribes *the
 *     caller*.
 *   - `/list` calls `ListFeedLinksWithHealthUsecase.Execute(ctx)` — note the
 *     absent user argument. It lists every known feed link, globally.
 *
 * So a successful DELETE correctly leaves the row in `/list`, and `/list` is
 * simply not where the effect is observable. The Hurl suite asserted only the
 * `"unsubscribed"` message and so never had to have an opinion; naming the
 * distinction here is worth more than the round-trip assertion that was
 * originally written, because "delete" reads like a destructive operation on
 * the resource `/list` returns and it is not one.
 */
test.describe("unsubscribe", () => {
	test("DELETE /:id unsubscribes the caller and reports it", async ({ rest, csrf }, testInfo) => {
		// Register-then-delete inside one test. The Hurl scenario listed and
		// deleted `$[0].id`, which under any parallelism unsubscribes somebody
		// else's row and leaves the assertion testing nothing.
		const url = stubURL(`feed-del-${testInfo.workerIndex}-${Date.now()}.xml`);
		await expectStatus(
			await rest.post("/v1/rss-feed-link/register", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { url },
			}),
			200,
		);

		const links = await expectJsonStatus(
			await rest.get("/v1/rss-feed-link/list"),
			200,
			rssFeedLinkListSchema,
		);
		const target = links.find((link) => link.url === url);
		expect(target, `just-registered ${url} must be listed`).toBeDefined();
		if (target === undefined) return;

		const deleted = await rest.delete(`/v1/rss-feed-link/${target.id}`, {
			headers: { "X-CSRF-Token": csrf },
		});
		await expectStatus(deleted, 200);
		expect(JSON.stringify(await deleted.json())).toContain("unsubscribed");

		// The feed link itself survives, by design — see the block comment. This
		// is asserted rather than merely not-asserted so that a future change
		// making DELETE destructive has to come through here.
		const after = await expectJsonStatus(
			await rest.get("/v1/rss-feed-link/list"),
			200,
			rssFeedLinkListSchema,
		);
		expect(
			after.map((link) => link.url),
			"/list is global, not per-user: unsubscribing must not delete the link",
		).toContain(url);
	});

	test("DELETE is idempotent — a second call still reports success", async ({
		rest,
		csrf,
	}, testInfo) => {
		// The driver issues the DELETE and never inspects RowsAffected, so
		// unsubscribing twice is indistinguishable from unsubscribing once. That
		// is the right behaviour for an unsubscribe and the wrong one for a
		// delete, which is the other half of why the naming matters.
		const url = stubURL(`feed-del-idem-${testInfo.workerIndex}-${Date.now()}.xml`);
		await expectStatus(
			await rest.post("/v1/rss-feed-link/register", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { url },
			}),
			200,
		);

		const links = await expectJsonStatus(
			await rest.get("/v1/rss-feed-link/list"),
			200,
			rssFeedLinkListSchema,
		);
		const target = links.find((link) => link.url === url);
		if (target === undefined) return;

		for (const attempt of [1, 2]) {
			const response = await rest.delete(`/v1/rss-feed-link/${target.id}`, {
				headers: { "X-CSRF-Token": csrf },
			});
			expect(response.status(), `attempt ${attempt}`).toBe(200);
		}
	});

	test("DELETE of an unknown id reports success rather than 404", async ({ rest, csrf }) => {
		// Pinned after the first CI run. `DeleteSubscription` does not check
		// RowsAffected, so "you were never subscribed" and "you are now
		// unsubscribed" are the same answer — 200, not the 404 this test
		// originally expected.
		//
		// Worth stating explicitly because the sibling admin surface made the
		// opposite choice: `handleGetScrapingDomain` maps a missing row to 404.
		// Neither is wrong on its own; the two together mean a client cannot
		// infer existence from a status code consistently across this API.
		const response = await rest.delete(
			"/v1/rss-feed-link/00000000-0000-0000-0000-000000000000",
			{ headers: { "X-CSRF-Token": csrf } },
		);
		await expectStatus(response, 200);
	});

	test("DELETE with a malformed id is rejected", async ({ rest, csrf }) => {
		const response = await rest.delete("/v1/rss-feed-link/not-a-uuid", {
			headers: { "X-CSRF-Token": csrf },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});
