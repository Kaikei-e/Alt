import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { testToken, uuid } from "../../_shared/ids.js";
import {
	backfillAcceptedSchema,
	notImplementedSchema,
	restErrorSchema,
} from "../src/schemas.js";

/**
 * The Echo REST surface — entirely new coverage.
 *
 * The Hurl suite's README declared every `/v1/rag/*` and `/internal/rag/*`
 * route out of scope for Phase 1 because they "need news-creator (Ollama),
 * search-indexer, rerank-external, alt-backend". That is true of the *happy
 * paths* and only of the happy paths: seven routes are registered
 * (openapi/server.gen.go:229-233 plus the two hand-mounted ones at
 * main.go:121-122), and each one's bind failure and validation gates run
 * entirely in-process, before any upstream is touched. Leaving them uncovered
 * meant `RegisterHandlers` could stop being called and nothing would go red.
 *
 * The discriminator throughout is 404: a route that answers *its own* 400 has
 * resolved through Echo's router into the handler, which is what a DI or
 * registration regression takes away. Every probe below is deliberately shaped
 * to short-circuit before `indexUsecase` / `answerUsecase` / `retrieveUsecase`
 * run, because in this slice those point at `http://127.0.0.1:9`
 * (compose.staging.yaml) and reaching them costs a 10-30s timeout rather than
 * an assertion.
 */

/**
 * Every route with a `ctx.Bind` at the top of its handler.
 *
 * `/internal/rag/index/delete` is deliberately absent: it returns 501 without
 * binding anything, so it cannot be probed the same way. Its registration is
 * covered by its own test below.
 */
const BINDING_ROUTES = [
	"/internal/rag/index/upsert",
	"/v1/rag/answer",
	"/v1/rag/answer/stream",
	"/v1/rag/retrieve",
	"/internal/rag/backfill",
	"/v1/rag/morning-letter",
] as const;

test.describe("REST route registration", () => {
	for (const route of BINDING_ROUTES) {
		test(`POST ${route} is mounted`, { tag: "@contract" }, async ({ rest }) => {
			// A syntactically invalid JSON body makes `ctx.Bind` fail in every one
			// of these handlers, and each answers its own
			// `400 {"error":"invalid request"}` before touching a usecase. The
			// 400 is what proves the request reached a handler.
			//
			// 404 would mean `openapi.RegisterHandlers(e, handler)` never ran (or
			// the hand-mounted `e.POST` lines at main.go:121-122 were lost), and
			// 405 would mean the path exists under a different verb. Both are
			// wiring failures that no unit test sees. A 5xx would mean the bind
			// guard is gone and the body reached the pipeline, which in this
			// slice means an unreachable upstream — also a regression, and also
			// caught here.
			//
			// For `/v1/rag/retrieve` this is the *only* in-process assertion
			// available: unlike the two answer routes it validates nothing of its
			// own, so an empty query goes straight into the retrieval graph and
			// the unreachable embedder. The bind guard is the whole of what can
			// be asserted without a 30-second timeout.
			const response = await rest.post(route, {
				headers: { "Content-Type": "application/json" },
				data: "{ this is not json",
			});
			await expectJsonStatus(response, 400, restErrorSchema);
		});
	}
});

test.describe("declared-but-unimplemented routes", () => {
	test(
		"POST /internal/rag/index/delete answers 501, not a silent success",
		{ tag: "@contract" },
		async ({ rest }) => {
			// New coverage. `DeleteIndex` is a one-line stub
			// (rag_http/handler.go:166) that the OpenAPI spec still advertises. A
			// caller that read a 2xx here would believe a tombstone had been
			// written and stop retrying; 501 is what keeps "not built yet"
			// distinguishable from "done".
			const response = await rest.post("/internal/rag/index/delete", {
				data: { article_id: uuid(), user_id: uuid() },
			});
			await expectJsonStatus(response, 501, notImplementedSchema);
		},
	);
});

test.describe("query validation", () => {
	// Both answer routes and the morning-letter route reject an empty query
	// before constructing any usecase input, so these never reach the LLM.
	const QUERY_REQUIRED = [
		"/v1/rag/answer",
		"/v1/rag/answer/stream",
		"/v1/rag/morning-letter",
	] as const;

	for (const route of QUERY_REQUIRED) {
		test(`POST ${route} rejects an empty query`, { tag: "@contract" }, async ({ rest }) => {
			// New coverage. The check is a literal `req.Query == ""` in each
			// handler (handler.go:229 / :333 / :574). Without it the request runs
			// the whole retrieval + generation pipeline on an empty string and
			// fails 30 seconds later as a 500 — the difference between a client
			// bug reported instantly and one that looks like an outage.
			const response = await rest.post(route, { data: {} });
			const body = await expectJsonStatus(response, 400, restErrorSchema);
			expect(body.error).toBe("query is required");
		});
	}
});

test.describe("backfill enqueue", () => {
	test("rejects a body with no article_id", { tag: "@contract" }, async ({ rest }) => {
		const body = await expectJsonStatus(
			await rest.post("/internal/rag/backfill", { data: {} }),
			400,
			restErrorSchema,
		);
		expect(body.error).toBe("missing article_id");
	});

	test("rejects an article_id that is not a UUID", { tag: "@contract" }, async ({ rest }) => {
			// The handler's own comment says why this gate exists: `article_id`
			// flows unvalidated into the job payload otherwise, and
			// `processBackfillArticle` only type-asserts it to string
			// (worker.go:139) — so a malformed id would be discovered by a
			// background worker, minutes later, as an opaque job failure.
			const body = await expectJsonStatus(
				await rest.post("/internal/rag/backfill", {
					data: { article_id: "not-a-uuid", title: "t", body: "b" },
				}),
				400,
				restErrorSchema,
			);
			expect(body.error).toBe("invalid article_id");
	});

	test("rejects a missing title", { tag: "@contract" }, async ({ rest }) => {
		const body = await expectJsonStatus(
			await rest.post("/internal/rag/backfill", { data: { article_id: uuid(), body: "b" } }),
			400,
			restErrorSchema,
		);
		// Each gate has its own message (handler.go:398-409). Asserting the
		// literal is what keeps them distinguishable: a refactor that collapsed
		// all four into one "invalid request" would still return 400 and still
		// pass a status-only test, while telling the caller nothing.
		expect(body.error).toBe("missing title");
	});

	test("rejects a missing body", { tag: "@contract" }, async ({ rest }) => {
		const response = await rest.post("/internal/rag/backfill", {
			data: { article_id: uuid(), title: "t" },
		});
		const body = await expectJsonStatus(response, 400, restErrorSchema);
		expect(body.error).toBe("missing body");
	});

	test(
		"enqueues a well-formed job and hands back its id",
		{ tag: "@contract" },
		async ({ rest, workerTag }, testInfo) => {
			// New coverage, and the only test in the suite that writes through the
			// REST surface. It exercises the one write path that needs no upstream
			// at all: `jobRepo.Enqueue` is a plain INSERT into `rag_jobs`
			// (handler.go:433), and the row is the response.
			//
			// The article id is minted per test and the title carries the worker
			// token, so N workers enqueue N distinct rows — nothing here is shared
			// state.
			//
			// What is deliberately NOT asserted is what happens to the job
			// afterwards. The JobWorker will pick it up, try to embed against
			// `http://127.0.0.1:9`, fail, and grow a process-wide exponential
			// backoff (worker.go:126) that every other enqueued job then waits
			// behind. Asserting a terminal status would make one test's result
			// depend on how many others ran first — which is precisely the kind of
			// coupling this migration exists to remove.
			const response = await rest.post("/internal/rag/backfill", {
				data: {
					article_id: uuid(),
					title: `rag-e2e-${testToken(testInfo.workerIndex, testInfo.title)}`,
					body: `Enqueued by the rag-orchestrator Playwright suite (${workerTag}).`,
					url: "https://example.invalid/rag-e2e-backfill",
				},
			});
			// 202, not 200: the work has not happened yet, and a client that read
			// this as completion would never poll.
			const body = await expectJsonStatus(response, 202, backfillAcceptedSchema);
			expect(body.status).toBe("queued");
		},
	);
});

test.describe("embedder override is an allowlist, not a hint", () => {
	test(
		"X-Embedder-URL pointing off the allowlist is rejected outright",
		{ tag: "@authz" },
		async ({ rest }) => {
			// New coverage for an SSRF guard the Hurl suite never reached.
			// `RAG_EMBEDDER_ALLOWED_OVERRIDE_URLS` defaults to exactly
			// `http://backfill-hyperboost:11434` (config.go:349) and
			// compose.staging.yaml does not override it, so any other origin must
			// fail the exact-match check in `isAllowedEmbedderOverride`
			// (handler.go:126).
			//
			// The rejection has to be *loud*: the handler's own comment explains
			// that silently falling back to the default embedder would look
			// identical to a successful override from the caller's side, hiding
			// the fact that the guard fired. That is why the assertion is on the
			// 400 and its message rather than merely "the request did not reach
			// the attacker's host".
			//
			// It also short-circuits before `indexUsecase.Upsert`, which is what
			// makes this probe fast in a slice whose embedder is unreachable.
			const response = await rest.post("/internal/rag/index/upsert", {
				headers: { "X-Embedder-URL": "http://attacker.invalid:11434" },
				data: { article_id: uuid(), title: "t", body: "b", url: "https://example.invalid/a" },
			});
			const body = await expectJsonStatus(response, 400, restErrorSchema);
			expect(body.error).toBe("X-Embedder-URL origin not allowed");
		},
	);

	test(
		"a host that merely starts with an allowed name is still rejected",
		{ tag: "@authz" },
		async ({ rest }) => {
			// `.claude/rules/security-boundaries.md` requires exact-match
			// allowlisting, and `normalizeOrigin` (handler.go:97) is what
			// implements it. This is the case its doc comment names verbatim:
			// `http://backfill-hyperboost.evil.com:11434` must not pass as
			// `http://backfill-hyperboost:11434`. A prefix or substring check —
			// the natural way to write this by hand — passes the test above and
			// fails this one.
			const response = await rest.post("/internal/rag/index/upsert", {
				headers: { "X-Embedder-URL": "http://backfill-hyperboost.evil.com:11434" },
				data: { article_id: uuid(), title: "t", body: "b", url: "https://example.invalid/a" },
			});
			const body = await expectJsonStatus(response, 400, restErrorSchema);
			expect(body.error).toBe("X-Embedder-URL origin not allowed");
		},
	);

	test(
		"a non-HTTP scheme in X-Embedder-URL is rejected",
		{ tag: "@authz" },
		async ({ rest }) => {
			// `normalizeOrigin` accepts only http/https with a non-empty host, so
			// `file:` and `javascript:` never reach the allowlist lookup at all.
			// Asserting one of them pins the scheme gate independently of the
			// allowlist contents — otherwise an empty allowlist would make every
			// test in this describe pass for the wrong reason.
			const response = await rest.post("/internal/rag/index/upsert", {
				headers: { "X-Embedder-URL": "file:///etc/passwd" },
				data: { article_id: uuid(), title: "t", body: "b", url: "https://example.invalid/a" },
			});
			const body = await expectJsonStatus(response, 400, restErrorSchema);
			expect(body.error).toBe("X-Embedder-URL origin not allowed");
		},
	);
});

test.describe("the router has no catch-all", () => {
	test(
		"an unregistered REST path is a 404, not a 400 from a catch-all",
		{ tag: "@contract" },
		async ({ rest }) => {
			// The control for every "is mounted" assertion above. Echo has no
			// catch-all here, so an unknown path must 404 — if it did not, the
			// registration tests would be satisfied by a router that answers
			// everything.
			await expectStatus(await rest.post("/v1/rag/no-such-route", { data: {} }), 404);
		},
	);
});
