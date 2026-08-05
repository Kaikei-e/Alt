import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import { augurAnswerSchema, secureErrorSchema } from "../src/schemas.js";

/**
 * Augur (RAG) surface — the port of `60-augur-rag.hurl`.
 *
 * Request shapes fixed against the real Go code: the handler reads query param
 * `q` (augur_handler.go:30), not `query`; AnswerRequest binds
 * `messages: [{role, content}]` + `stream`, not a bespoke
 * `query`/`conversation_id` shape.
 *
 * KNOWN BUG (recorded, not fixed — the deps-stub is out of scope for this
 * suite): the alt-backend → rag-orchestrator REST client calls
 * `POST /v1/rag/retrieve` (rag_gateway's generated operationPath), but the
 * deps-stub only implements `/v1/context` and the Connect-RPC
 * `/alt.augur.v2.AugurService/RetrieveContext` — no route for
 * `/v1/rag/retrieve`. The request falls through to the stub's catch-all
 * (200 + `{"status":"stub-noop"}`), which decodes into a zero-value
 * RetrieveResponse; augur_adapter.go:72-74 rejects a nil `Contexts` field as
 * "rag-orchestrator returned empty response", and RetrieveContext maps that to
 * a generic 500. Same class of stub-path drift as morning-letter.
 */

test.describe("GET /v1/rag/context", () => {
	test("stub drift surfaces as a typed UNKNOWN_ERROR", async ({ rest }) => {
		const body = await expectJsonStatus(
			await rest.get("/v1/rag/context?q=ai"),
			500,
			secureErrorSchema,
		);
		expect(body.error.code).toBe("UNKNOWN_ERROR");
	});

	test("a missing q is a 400 before any upstream call", async ({ rest }) => {
		// New coverage, and the one deterministic branch of this handler that
		// does not depend on the stub: `q` is validated first, so this answers
		// 400 whether or not rag-orchestrator is reachable.
		const response = await rest.get("/v1/rag/context");
		await expectStatus(response, 400);
		expect(await response.text()).toContain("q");
	});
});

/**
 * ATTENTION — this endpoint pair is **not authenticated**.
 *
 * `RegisterAugurRoutes(e, v1, container)` registers
 * `g.GET("/rag/context", …)` on the bare `/v1` group and
 * `e.POST("/sse/v1/rag/answer", …)` on the root Echo instance. Neither carries
 * `authMiddleware.RequireAuth()`, unlike every other `/v1` group in
 * routes.go — so both reach the RAG orchestrator for any caller, with no JWT,
 * no tenant scoping, and no admin check.
 *
 * The Hurl suite sent a JWT on both calls and never probed the negative, so
 * this was invisible. The tests below pin the *current* behaviour rather than
 * the desired one, deliberately: changing who may call a live endpoint is a
 * product decision, not a test-suite decision. When the routes are moved under
 * RequireAuth, these two tests fail — which is the point. Delete them then and
 * add the two lines to the PROTECTED_ROUTES table in tests/auth.spec.ts.
 */
test.describe("augur authentication gap (pinned, not endorsed)", () => {
	test("GET /v1/rag/context answers without a JWT", async ({ restAnon }) => {
		const response = await restAnon.get("/v1/rag/context?q=ai");
		expect(
			response.status(),
			"if this is now 401, the augur routes were put behind RequireAuth — " +
				"move them into tests/auth.spec.ts's PROTECTED_ROUTES and delete this test",
		).not.toBe(401);
	});

	test("POST /sse/v1/rag/answer answers without a JWT", async ({ restAnon, csrf }) => {
		const response = await restAnon.post("/sse/v1/rag/answer", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { messages: [{ role: "user", content: "hello" }], stream: false },
		});
		expect(
			response.status(),
			"if this is now 401, the augur routes were put behind RequireAuth — " +
				"move them into tests/auth.spec.ts's PROTECTED_ROUTES and delete this test",
		).not.toBe(401);
	});
});

test.describe("POST /sse/v1/rag/answer", () => {
	test("a non-streaming answer returns 200 with an empty answer", async ({ rest, csrf }) => {
		// Same stub-path drift as above: the real client calls
		// `POST /v1/rag/answer` (not the stub's `/v1/answer`) and also misses.
		// Unlike RetrieveContext, Answer's non-streaming branch treats a missing
		// `JSON200.Answer` as "no answer" rather than an error
		// (augur_adapter.go:157-164 closes the channel with nothing sent instead
		// of returning an error), so the handler still responds 200 with an
		// empty answer rather than 500.
		const body = await expectJsonStatus(
			await rest.post("/sse/v1/rag/answer", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: {
					messages: [{ role: "user", content: "summarize today's ai highlights" }],
					stream: false,
				},
			}),
			200,
			augurAnswerSchema,
		);
		expect(body.answer).toBe("");
	});

	test("a body with no user message is rejected", async ({ rest, csrf }) => {
		// New coverage. The handler walks the message list backwards looking for
		// the last `role: "user"` entry and 400s when there is none — a branch
		// the frontend hits whenever it replays an assistant-only transcript.
		const response = await rest.post("/sse/v1/rag/answer", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { messages: [{ role: "assistant", content: "hi" }], stream: false },
		});
		await expectStatus(response, 400);
	});

	test("an empty message list is rejected", async ({ rest, csrf }) => {
		const response = await rest.post("/sse/v1/rag/answer", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { messages: [], stream: false },
		});
		await expectStatus(response, 400);
	});

	test("is CSRF-protected despite living outside /v1", async ({ rest }) => {
		// New coverage. The route is mounted on the root Echo instance, not the
		// /v1 group, and `isCSRFProtectedEndpoint` protects by default rather
		// than by allowlist — which is the only reason this path is covered at
		// all. A change to opt-in protection would silently expose it.
		const response = await rest.post("/sse/v1/rag/answer", {
			headers: { "Content-Type": "application/json" },
			data: { messages: [{ role: "user", content: "hello" }], stream: false },
		});
		await expectStatus(response, 403);
	});
});
