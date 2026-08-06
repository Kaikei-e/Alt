import { test, expect, summarizeBody, ARTICLE_CONTENT } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { fastapiErrorSchema, summarizeResponseSchema } from "../src/schemas.js";

/**
 * `POST /api/v1/summarize` (non-streaming) — the port of
 * `03-summarize-non-stream.hurl` plus the guard rails from
 * `12-validation-errors.hurl` that belong to this endpoint.
 *
 * This is pre-processor's and acolyte's path: an article in, a Japanese
 * summary out. The Hurl scenario checked four JSONPaths on a fixed
 * `article_id`; here the whole `SummarizeResponse` envelope is asserted, and
 * the `article_id` is minted per test so the echo actually proves something.
 */
test.describe("summarize", () => {
	test("returns a summary and echoes the caller's article_id @smoke @contract", async ({
		api,
		seed,
	}) => {
		const response = await api.post("/api/v1/summarize", {
			data: summarizeBody(seed.articleId),
		});
		const body = await expectJsonStatus(response, 200, summarizeResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		// The Hurl scenario asserted `article_id == "hurl-e2e-article-normal"`, a
		// constant baked into both the request fixture and the expectation — so a
		// handler that ignored the request and returned a hard-coded id would
		// have passed. A per-test id cannot be satisfied that way.
		expect(body.article_id).toBe(seed.articleId);

		// `model` is the metadata the usecase read back off the LLM response, so
		// pinning it to the model this slice configured proves the summary came
		// from the upstream we pointed at rather than from a fallback path.
		expect(body.model).toBe(env.stubModel);
	});

	test("rejects content shorter than the 100-character minimum @contract", async ({
		api,
		seed,
	}) => {
		// `summarize_handler` refuses before the LLM call: content under 100
		// characters after `.strip()` is a 400 with a prose `detail`
		// (handler/summarize_handler.py:67-83). This guard is the whole reason
		// pre-processor can post freely without pre-filtering, and it must stay a
		// 400 — a 422 would mean the check migrated into Pydantic and started
		// answering a different envelope.
		const response = await api.post("/api/v1/summarize", {
			data: { article_id: seed.articleId, content: "短すぎる本文", stream: false, priority: "low" },
		});
		const body = await expectJsonStatus(response, 400, fastapiErrorSchema);
		expect(typeof body.detail === "string" ? body.detail : "").toMatch(/content is too short/i);
	});

	test("rejects whitespace-only content as too short @contract", async ({ api, seed }) => {
		// New coverage. The guard measures `request.content.strip()`, but
		// `SummarizeRequest.content` is only `min_length=1` — so 200 spaces
		// satisfies Pydantic and must still be caught by the handler. Without
		// this, a refactor that dropped the `.strip()` would send whitespace to
		// the GPU and the Hurl suite's short-content case would still pass.
		const response = await api.post("/api/v1/summarize", {
			data: { article_id: seed.articleId, content: " ".repeat(200), stream: false, priority: "low" },
		});
		await expectStatus(response, 400);
	});

	test("rejects an unknown priority @contract", async ({ api, seed }) => {
		// New coverage. `priority` is `pattern="^(high|low)$"` on a strict+frozen
		// model (domain/models.py SummarizeRequest), and it decides whether the
		// request takes an RT-reserved slot or queues as best-effort. A silently
		// coerced value would put real-time traffic in the batch pool, which is
		// precisely the starvation the HybridPrioritySemaphore exists to prevent.
		const response = await api.post("/api/v1/summarize", {
			data: { article_id: seed.articleId, content: ARTICLE_CONTENT, priority: "urgent" },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a non-string content under Pydantic strict mode @contract", async ({
		api,
		seed,
	}) => {
		// New coverage. `StrictFrozenModel` sets `strict=True`, so Pydantic does
		// *not* coerce — an int where a string belongs is a 422, not a stringified
		// article. Callers rely on that: a JSON encoder bug upstream should fail
		// loudly here rather than have news-creator summarize the digits.
		const response = await api.post("/api/v1/summarize", {
			data: { article_id: seed.articleId, content: 12345, stream: false },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a missing article_id @contract", async ({ api }) => {
		// New coverage. `article_id` is `min_length=1` and threads through every
		// log line as ADR-98 business context; a request without one would
		// produce an unattributable summary.
		const response = await api.post("/api/v1/summarize", {
			data: { content: ARTICLE_CONTENT, stream: false },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a malformed JSON body @contract", async ({ api }) => {
		// Port of `12-validation-errors.hurl` case 3. FastAPI's request-body
		// parser answers its own 422 before any handler code runs, so this pins
		// the outermost error envelope callers see for a transport-level mistake.
		const response = await api.post("/api/v1/summarize", {
			headers: { "Content-Type": "application/json" },
			data: "{not valid json}",
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("GET is not allowed on the summarize endpoint @contract", async ({ api }) => {
		// New coverage. The route is registered `POST`-only. 405 (not 404) is the
		// assertion that matters: a 404 would mean the router was never included
		// at all — the DI-wiring failure mode CLAUDE.md rule 8 is about — while
		// 405 proves the path resolved and only the verb was wrong.
		await expectStatus(await api.get("/api/v1/summarize"), 405);
	});
});
