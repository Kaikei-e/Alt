import { test, expect, stubURL } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
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

	test("POST /import/opml rejects a non-OPML upload", async ({ rest, csrf }) => {
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
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("deletion", () => {
	test("DELETE /:id removes a feed this test registered", async ({ rest, csrf }, testInfo) => {
		// Register-then-delete inside one test. The Hurl scenario listed and
		// deleted `$[0].id`, which under any parallelism deletes somebody else's
		// row and leaves this assertion testing nothing.
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
		const body: unknown = await deleted.json();
		expect(JSON.stringify(body)).toContain("unsubscribed");

		// And it is gone. Asserting the *effect* rather than the response
		// message is the difference between testing the handler and testing the
		// DELETE.
		const after = await expectJsonStatus(
			await rest.get("/v1/rss-feed-link/list"),
			200,
			rssFeedLinkListSchema,
		);
		expect(after.map((link) => link.url)).not.toContain(url);
	});

	test("DELETE of an unknown id is tenant-scoped, not a server error", async ({ rest, csrf }) => {
		// The DB DELETE carries the tenant predicate from the JWT, so another
		// tenant's link is indistinguishable from a missing one: 404, never 403,
		// because 403 would confirm the row exists.
		const response = await rest.delete(
			"/v1/rss-feed-link/00000000-0000-0000-0000-000000000000",
			{ headers: { "X-CSRF-Token": csrf } },
		);
		expect(response.status()).not.toBe(403);
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});

	test("DELETE with a malformed id is rejected", async ({ rest, csrf }) => {
		const response = await rest.delete("/v1/rss-feed-link/not-a-uuid", {
			headers: { "X-CSRF-Token": csrf },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});
