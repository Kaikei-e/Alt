import { test, expect, stubURL } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus, expectStatusIn } from "../src/http.js";
import { ZERO_UUID } from "../src/env.js";
import {
	articleTagsSchema,
	articlesByTagSchema,
	fullCursorPageSchema,
} from "../src/schemas.js";

/**
 * Articles surface — the port of `30-articles.hurl`.
 *
 * Request shapes pinned against the real Go handlers, all three of which were
 * wrong in an earlier generation and hidden by a blanket
 * `status >= 200; status < 600` assertion:
 *   - `/by-tag` reads `tag_name` / `tag_id`, not a generic `tag`.
 *   - `/search` reads `q`, not `query`.
 *   - `/archive` binds `{feed_url, title}`, not `{article_id, archived}`.
 *
 * The two entries that fetch an article body cannot succeed against the
 * staging stack. The handler-level allowlist (FEED_ALLOWED_HOSTS) admits
 * stub.invalid, but the outbound fetch is guarded a second time by the SSRF
 * checker, which rejects it: stub.invalid resolves to a Docker-internal
 * address and the guard reports DNS_REBINDING_BLOCKED. Retries then hit the
 * per-host rate limiter and fail the same way. Their bands exclude 2xx (a
 * success would mean the guard or the stub host changed and the content
 * assertions should come back) and 4xx (which would mean the URL param or
 * allowlist regressed).
 */

test.describe("GET /v1/articles/fetch/cursor", () => {
	test("returns a well-formed cursor page", async ({ rest }) => {
		await expectJsonStatus(
			await rest.get("/v1/articles/fetch/cursor?limit=10"),
			200,
			fullCursorPageSchema,
		);
	});

	test("rejects a malformed cursor", async ({ rest }) => {
		// New coverage — the Hurl file only checked the happy path here, even
		// though the feed cursor's malformed-cursor branch was covered. Two
		// handlers, one shared expectation.
		const response = await rest.get("/v1/articles/fetch/cursor?cursor=not-rfc3339");
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("GET /v1/articles/fetch/content", () => {
	test("is blocked by the SSRF guard for the stub host", async ({ rest }) => {
		const url = stubURL("article-1.html");
		const response = await rest.get(
			`/v1/articles/fetch/content?url=${encodeURIComponent(url)}`,
		);
		await expectStatusIn(response, [500, 503, 504]);
	});

	test("rejects a missing url parameter", async ({ rest }) => {
		const response = await rest.get("/v1/articles/fetch/content");
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("GET /v1/articles/by-tag", () => {
	test("an unregistered tag yields an empty, non-error result set", async ({ rest }) => {
		// `tag_name` is the correct query param (article_handlers.go:251); a tag
		// that was never registered against any feed just yields
		// ArticlesByTagResponse{Articles: []}.
		const body = await expectJsonStatus(
			await rest.get("/v1/articles/by-tag?tag_name=ai&limit=5"),
			200,
			articlesByTagSchema,
		);
		expect(body.has_more).toBe(false);
		expect(body.articles.length).toBe(0);
	});

	test("rejects a request with neither tag_name nor tag_id", async ({ rest }) => {
		const response = await rest.get("/v1/articles/by-tag?limit=5");
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("GET /v1/articles/:id/tags", () => {
	test("an unknown article id returns an empty tag list, not a 404", async ({ rest }) => {
		// FetchArticleTagsUsecase does no existence check, so this is a 200 with
		// an empty array. Pinning it makes a future 404 a deliberate change.
		const body = await expectJsonStatus(
			await rest.get(`/v1/articles/${ZERO_UUID}/tags`),
			200,
			articleTagsSchema,
		);
		expect(body.article_id).toBe(ZERO_UUID);
		expect(body.tags.length).toBe(0);
	});
});

test.describe("GET /v1/articles/search", () => {
	test("returns the stub's deterministic empty hit set", async ({ rest }) => {
		// `q` is the correct query param (article_handlers.go:142). The deps-stub
		// always answers SearchArticles with an empty hit set
		// (_EMPTY_SEARCH_RESPONSE), so the search usecase returns `[]` every run.
		const response = await rest.get("/v1/articles/search?q=ai&limit=5");
		await expectStatus(response, 200);
		const body: unknown = await response.json();
		expect(Array.isArray(body)).toBe(true);
		expect((body as unknown[]).length).toBe(0);
	});

	test("rejects an empty q", async ({ rest }) => {
		// New coverage: ValidationMiddleware's validateArticleSearch owns this
		// route and had no test at all.
		const response = await rest.get("/v1/articles/search?q=&limit=5");
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});

	test("ignores `limit` entirely — it is not a parameter of this endpoint", async ({ rest }) => {
		// Pinned after the first CI run disproved the assumption behind this
		// test. `handleSearchArticles` reads `q` and nothing else: there is no
		// limit parsing, and ValidationMiddleware's `validateArticleSearch` does
		// not apply PaginationValidator to it either. So `limit=1001` is neither
		// honoured nor rejected — it is silently dropped.
		//
		// That is worth pinning rather than deleting: `/v1/articles/fetch/cursor`
		// and the feed cursors all take a `limit`, so a caller reasonably assumes
		// this one does too and gets a full result set instead of a page.
		const response = await rest.get("/v1/articles/search?q=ai&limit=1001");
		await expectStatus(response, 200);
		expect(Array.isArray(await response.json())).toBe(true);
	});
});

test.describe("POST /v1/articles/archive", () => {
	test("fetches the article body and hits the SSRF guard", async ({ rest, csrf }, testInfo) => {
		// ArchiveArticleRequest requires `feed_url`, not `article_id`/`archived`;
		// `title` is optional. Archiving fetches the body before saving, so it
		// hits the same guard as /fetch/content and cannot reach its 200 here.
		// 4xx stays excluded so a regression back to the wrong request shape
		// (which only ever reached the 400 branch) fails the run.
		const response = await rest.post("/v1/articles/archive", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: {
				feed_url: stubURL(`article-archive-${testInfo.workerIndex}-${Date.now()}.html`),
				title: "playwright-e2e archive test",
			},
		});
		await expectStatusIn(response, [500, 503, 504]);
	});

	test("rejects the legacy article_id request shape", async ({ rest, csrf }) => {
		// The regression fence for the bug this file's header describes: the old
		// body bound nothing and only ever reached the 400 branch. If it starts
		// succeeding, the binding changed and every band above needs revisiting.
		const response = await rest.post("/v1/articles/archive", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { article_id: ZERO_UUID, archived: true },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});
