import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import {
	expandQueryResponseSchema,
	fastapiErrorSchema,
	planQueryResponseSchema,
} from "../src/schemas.js";

/**
 * The rag-orchestrator surface — the port of `07-expand-query.hurl` and
 * `09-plan-query.hurl`, plus `/v1/rerank`, which the Hurl suite left entirely
 * uncovered.
 *
 * Both endpoints turn one free-text query into structured retrieval
 * instructions for Ask Augur. `plan-query` deliberately does *not* pass a
 * `format` field (Ollama #10929: thinking + format yields empty content), so
 * the JSON arrives as a string inside the chat content and `_extract_json` has
 * to find it — which is why the plan's shape is worth asserting field by field
 * rather than trusting a 200.
 */
test.describe("expand-query", () => {
	test("expands a query and echoes the original @smoke @contract", async ({ api, seed }) => {
		const query = `OpenAI 最新の研究 ${seed.token}`;
		const response = await api.post("/api/v1/expand-query", {
			data: { query, japanese_count: 2, english_count: 2, priority: "high" },
		});
		const body = await expectJsonStatus(response, 200, expandQueryResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		expect(body.original_query).toBe(query);
		expect(body.model).toBe(env.stubModel);

		// The usecase splits the model's output on newlines, drops
		// instruction-leak preambles and **dedupes**. rag-orchestrator fans one
		// vector search out per expansion, so a duplicate is wasted retrieval
		// budget and a skewed score distribution. `expanded_queries count >= 1`
		// — the Hurl assertion — is true of a list that is the same string three
		// times over.
		expect(new Set(body.expanded_queries).size).toBe(body.expanded_queries.length);

		// And no expansion may be the input echoed back: that is the classic
		// small-model failure the leak filter exists to catch, and it would make
		// expansion a no-op while still answering a well-formed envelope.
		expect(body.expanded_queries).not.toContain(query);
	});

	test("expand-query rejects an empty query @contract", async ({ api }) => {
		// New coverage. `ExpandQueryRequest.query` is `min_length=1`.
		const response = await api.post("/api/v1/expand-query", { data: { query: "" } });
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects counts outside their bounds @contract", async ({ api, seed }) => {
		// New coverage. `japanese_count` is `ge=0, le=5` and `english_count`
		// `ge=0, le=10` (domain/models.py). The ceilings are what stop a caller
		// from asking for 500 expansions and pinning the single semaphore slot
		// for the length of the generation.
		const tooManyJa = await api.post("/api/v1/expand-query", {
			data: { query: `bounds probe ${seed.token}`, japanese_count: 99 },
		});
		await expectJsonStatus(tooManyJa, 422, fastapiErrorSchema);

		const negativeEn = await api.post("/api/v1/expand-query", {
			data: { query: `bounds probe ${seed.token}`, english_count: -1 },
		});
		await expectJsonStatus(negativeEn, 422, fastapiErrorSchema);
	});
});

test.describe("plan-query", () => {
	test("returns a structured retrieval plan @smoke @contract", async ({ api, seed }) => {
		const query = `OpenAI の最新発表について教えて ${seed.token}`;
		const response = await api.post("/api/v1/plan-query", {
			data: { query, priority: "high" },
		});
		const body = await expectJsonStatus(response, 200, planQueryResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		expect(body.original_query).toBe(query);
		expect(body.model).toBe(env.stubModel);

		// `should_clarify` is documented as "ALMOST ALWAYS false" and true only
		// for a bare ambiguous phrase with no history to resolve from. A concrete
		// topical query that comes back asking for clarification means Augur
		// stops and interrogates the user instead of retrieving — the single
		// most user-visible way this planner can regress.
		expect(body.plan.should_clarify).toBe(false);

		// `resolved_query` is the string the retrieval layer actually searches
		// with; an empty or whitespace one would send Augur to the vector store
		// with nothing. `min(1)` in the schema covers empty, this covers blank.
		expect(body.plan.resolved_query.trim().length).toBeGreaterThan(0);

		// The enum membership of `intent` / `retrieval_policy` / `answer_format`
		// is asserted by `queryPlanSchema` above — those three fields are free
		// strings in Pydantic, so the schema is the only thing checking them, and
		// `expectJsonStatus` has already run it. Nothing to repeat here.
		//
		// What the schema cannot see is *which code path* produced a
		// schema-valid plan. `_plan_with_llm` swallows every JSONDecodeError,
		// ValidationError, TypeError and KeyError into `_fallback_plan` under a
		// 200 (plan_query_usecase.py:139-149), and `PlanQueryResponse.model` is
		// stamped from config rather than read off the reply
		// (plan_query_usecase.py:109-114), so a completely broken structured-
		// output round trip answers a perfect envelope naming the right model.
		// `_fallback_plan` is the one thing that identifies itself: it hard-codes
		// `reasoning="Fallback: LLM planning failed, using original query
		// directly."` (plan_query_usecase.py:198-208). Refusing that prefix is
		// what makes this test fail when `_extract_json` stops finding the JSON
		// the chat content carries.
		expect(
			body.plan.reasoning,
			"the plan came from _fallback_plan — the LLM round trip failed and was " +
				"swallowed into a 200",
		).not.toMatch(/^Fallback:/);

		// And the fallback's other signature, in case its wording drifts: it
		// searches with nothing but the caller's own query, which would make
		// planning a no-op while still answering a well-formed plan.
		expect(body.plan.search_queries).not.toEqual([body.plan.resolved_query]);
	});

	test("plan-query rejects an unknown priority @contract", async ({ api, seed }) => {
		// New coverage. `PlanQueryRequest.priority` is `pattern="^(high|low)$"`
		// and defaults to `high` so real-time Augur queries take an RT-reserved
		// slot. A silently-accepted third value would land them in the
		// best-effort pool behind batch summarization — the exact starvation the
		// scheduler was built to prevent.
		const response = await api.post("/api/v1/plan-query", {
			data: { query: `priority probe ${seed.token}`, priority: "medium" },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("plan-query rejects an empty query @contract", async ({ api }) => {
		const response = await api.post("/api/v1/plan-query", { data: { query: "" } });
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});
});

test.describe("rerank", () => {
	/**
	 * New coverage. The Hurl suite deferred `/v1/rerank` entirely because
	 * `RerankUsecase` lazily downloads `BAAI/bge-reranker-v2-m3` (~568MB) from
	 * HuggingFace on first use, and the staging network is `internal: true`.
	 *
	 * That is a reason not to exercise the *model*, not a reason to leave the
	 * route unverified. `create_rerank_router` is one of nine
	 * `app.include_router` calls in main.py, and if it were dropped — or if the
	 * DI container failed to build `RerankUsecase` — rag-orchestrator would get
	 * a 404 that reads exactly like "no candidates matched". Pydantic validates
	 * `RerankRequest` before the handler body runs, so an invalid request proves
	 * the route resolved without ever touching the cross-encoder.
	 */
	test("the route is mounted and validates before loading any model @contract", async ({
		api,
		seed,
	}) => {
		// `candidates` is `min_length=1, max_length=100`.
		const response = await api.post("/v1/rerank", {
			data: { query: `rerank mount probe ${seed.token}`, candidates: [] },
		});
		expect(
			response.status(),
			"POST /v1/rerank answered 404 — create_rerank_router was never included " +
				"(main.py) or the DI container failed to build RerankUsecase. A 422 is " +
				"the expected 'mounted, and your request is invalid'.",
		).not.toBe(404);
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects top_k below 1 @contract", async ({ api, seed }) => {
		// `top_k` is `ge=1, le=100`. Same reasoning: a boundary the model never
		// has to load to enforce.
		const response = await api.post("/v1/rerank", {
			data: { query: `top_k probe ${seed.token}`, candidates: ["a"], top_k: 0 },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("GET is not allowed on rerank @contract", async ({ api }) => {
		await expectStatus(await api.get("/v1/rerank"), 405);
	});
});
