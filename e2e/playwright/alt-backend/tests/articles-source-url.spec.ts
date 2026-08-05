import { test, expect } from "../src/fixtures.js";
import { expectJson, expectStatus, expectStatusIn } from "../src/http.js";
import { ZERO_UUID } from "../src/env.js";
import { connectErrorSchema } from "../src/schemas.js";

/**
 * `ArticleService.GetArticleSourceURL` — the port of
 * `31-articles-source-url.hurl`.
 *
 * Background: ADR-879 added `actTargets[].source_url` so the frontend can
 * navigate the SPA reader with `?url=<external_https>`. When the producer-side
 * URL injection misses (legacy row, lookup miss, race), the projection's
 * source_url is empty and the Open CTA loses its target. The Loop UI exposes a
 * recovery affordance — "Open · resolve url" — that calls this RPC at click
 * time. It returns the canonical article URL for an article id, scoped to the
 * caller's tenant, and must:
 *
 *   - 200 with `{sourceUrl}` when the article belongs to the caller
 *   - 404 `not_found` when no row matches (missing, soft-deleted, other tenant)
 *   - 400 `invalid_argument` on a malformed UUID
 */

test.describe("GetArticleSourceURL", () => {
	test("a malformed UUID is invalid_argument", async ({ connect }) => {
		const response = await connect.post(
			"/alt.articles.v2.ArticleService/GetArticleSourceURL",
			{ data: { articleId: "not-a-uuid" } },
		);
		await expectStatus(response, 400);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe("invalid_argument");
	});

	test("an empty UUID is invalid_argument", async ({ connect }) => {
		// New coverage. An absent field decodes to the empty string, which is a
		// different branch from "present but unparseable" — and the one a
		// frontend actually produces when the projection has no id to send.
		const response = await connect.post(
			"/alt.articles.v2.ArticleService/GetArticleSourceURL",
			{ data: {} },
		);
		await expectStatus(response, 400);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe("invalid_argument");
	});

	test("an unknown article id is not_found", async ({ connect }) => {
		// The all-zero UUID is a stable "definitely-not-mine" probe.
		const response = await connect.post(
			"/alt.articles.v2.ArticleService/GetArticleSourceURL",
			{ data: { articleId: ZERO_UUID } },
		);
		await expectStatus(response, 404);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe("not_found");
	});

	test("a seeded article id resolves or is legitimately absent, never a server error", async ({
		connect,
	}) => {
		// No specific URL is pinned: the staging fixture set rotates and many
		// stacks have no seeded article that survives across runs. The RPC has
		// exactly two outcomes reachable here — not_found (404) or success (200
		// with a source URL) — while a genuine backend failure surfaces as 503.
		// Pinning to {200, 404} keeps the rotating fixture set from mattering
		// while still letting a real infra regression fail the run.
		const seed = process.env.ARTICLE_ID_SEED ?? "00000000-0000-0000-0000-000000000001";
		const response = await connect.post(
			"/alt.articles.v2.ArticleService/GetArticleSourceURL",
			{ data: { articleId: seed } },
		);
		await expectStatusIn(response, [200, 404]);

		if (response.status() === 200) {
			// If it did resolve, the URL has to be usable by the SPA reader —
			// which means absolute and http(s). An empty string here is the exact
			// failure the recovery affordance exists to repair, so it must not be
			// reported as a success.
			const body: unknown = await response.json();
			const sourceURL =
				typeof body === "object" && body !== null
					? (body as Record<string, unknown>)["sourceUrl"]
					: undefined;
			expect(typeof sourceURL).toBe("string");
			expect(String(sourceURL)).toMatch(/^https?:\/\//);
		}
	});

	test("is behind authentication", async ({ connectAnon }) => {
		await expectStatus(
			await connectAnon.post("/alt.articles.v2.ArticleService/GetArticleSourceURL", {
				data: { articleId: ZERO_UUID },
			}),
			401,
		);
	});
});
