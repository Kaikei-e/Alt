import { test, expect, genreLearningBody, triggerBody } from "../src/fixtures.js";
import {
	expectHeaderContains,
	expectJsonStatus,
	expectStatus,
} from "../../_shared/http.js";
import { expectRouteMounted } from "../src/assertions.js";
import {
	datasetErrorSchema,
	errorSchema,
	evaluationNotFoundSchema,
	genreLearningSchema,
	morningLetterSourcesSchema,
	searchRecapsSchema,
} from "../src/schemas.js";

/**
 * Every way a caller can be wrong — the port of `04-trigger-validation.hurl`,
 * plus the eleven other validation branches that existed in the source and
 * that no scenario touched.
 *
 * Two distinct rejection layers live in this file, and telling them apart is
 * most of its value:
 *
 *   axum's extractors reject before the handler body runs, with axum's own
 *   plain-text envelope: `Json` answers 415 for a missing/incorrect
 *   Content-Type, 400 for a JSON syntax error and 422 for a well-formed body
 *   of the wrong shape; `Query` and `Path` answer 400.
 *
 *   recap-worker's handlers reject with their own JSON envelope,
 *   `{"error": "…"}` (the `ErrorResponse` struct repeated across api/*.rs).
 *
 * A caller that switches on the body shape — which the SPA and alt-backend
 * both do — breaks silently when a field moves from one layer to the other:
 * making `term` an `Option<String>` with a manual check, or removing the
 * manual check and leaning on the extractor, both change the envelope without
 * changing the status. Asserting the layer, not just the status, is what
 * catches that.
 *
 * Everything here is parallel-safe: no test writes anything another reads, and
 * every "no such record" probe uses a fresh UUID or a per-test token.
 */

test.describe("trigger validation", () => {
	for (const window of ["7days", "3days"] as const) {
		test(`POST /v1/generate/recaps/${window} rejects an all-empty genres array @contract`, async ({
			api,
		}) => {
			// The Hurl port. `normalize_genres` trims, lowercases, dedupes and
			// drops empties (api/generate.rs:87-100); when nothing survives, the
			// handler answers 400 rather than falling back to the configured
			// defaults (api/generate.rs:53-58). The fallback is reserved for an
			// *absent* `genres` key — see the pipeline spec — and conflating the
			// two would silently run a full pipeline over every configured genre
			// for a caller who asked for none.
			//
			// The 3days case is new: the Hurl suite only exercised 7days, and
			// both routes funnel into the same `trigger_recap`, so this is the
			// assertion that they still do.
			const response = await api.post(`/v1/generate/recaps/${window}`, {
				data: triggerBody(["", "  "]),
			});
			const body = await expectJsonStatus(response, 400, errorSchema);
			expect(body.error).toBe("genres array must include at least one non-empty value");
			expectHeaderContains(response, "Content-Type", "application/json");
		});
	}

	test("POST /v1/generate/recaps/7days without a JSON content type answers 415 @contract", async ({
		api,
	}) => {
		// New coverage, and the cheapest possible proof that the route is
		// mounted: axum's `Json` extractor rejects on Content-Type before the
		// handler runs, so this resolves the route without spawning a pipeline
		// task. A 404 here would mean the `.route("/v1/generate/recaps/7days",
		// post(generate::trigger_7days))` line (api.rs:31) is gone.
		const response = await api.post("/v1/generate/recaps/7days", {
			headers: { "Content-Type": "text/plain" },
			data: "{}",
		});
		await expectRouteMounted(response, [415]);
	});

	test("POST /v1/generate/recaps/7days rejects malformed JSON with 400 @contract", async ({
		api,
	}) => {
		// axum's `JsonSyntaxError` branch. Distinct from the 422 below, and the
		// distinction is the contract: a client retrying on 400 and giving up on
		// 422 needs the two to mean "your bytes were broken" and "your bytes
		// were fine but your schema was not".
		const response = await api.post("/v1/generate/recaps/7days", {
			headers: { "Content-Type": "application/json" },
			data: "{ this is not json",
		});
		await expectStatus(response, 400);
	});

	test("POST /v1/generate/recaps/7days rejects a non-array genres field with 422 @contract", async ({
		api,
	}) => {
		// `genres: Option<Vec<String>>` — a number is a well-formed JSON body
		// that does not fit the target type, which is axum's `JsonDataError`
		// branch. Worth pinning because the natural "make it lenient" refactor
		// (`Option<Value>` plus a manual coercion) would turn this into a 202
		// and start a pipeline for a genre list nobody asked for.
		const response = await api.post("/v1/generate/recaps/7days", {
			data: { genres: 5 },
		});
		await expectStatus(response, 422);
	});

	test("GET /v1/generate/recaps/7days answers 405, not 404 @contract", async ({ api }) => {
		// The trigger is registered with `post(...)` only. 405 rather than 404
		// is what proves the *path* exists — which is what makes the 404
		// assertions in tests/topology.spec.ts mean something, and what would
		// catch a `get`/`post` swap that no unit test can see.
		const response = await api.get("/v1/generate/recaps/7days");
		await expectStatus(response, 405);
		expectHeaderContains(response, "Allow", "POST");
	});
});

test.describe("search validation", () => {
	test("GET /v1/recaps/search without a term is rejected by the extractor @contract", async ({
		api,
	}) => {
		// `SearchRecapsQuery.term` is a bare `String`, not an `Option`
		// (api/fetch.rs:216-221), so an absent `term` never reaches the handler
		// — axum's `Query` extractor rejects it. The status is the same 400 the
		// handler's own empty-term branch returns, but the *body* is not: this
		// one is axum's plain-text rejection, and the next test's is
		// `{"error": …}`. That asymmetry is a real thing a client sees, so it is
		// asserted rather than smoothed over.
		const response = await api.get("/v1/recaps/search");
		await expectStatus(response, 400);
		expectHeaderContains(response, "Content-Type", "text/plain");
	});

	test("GET /v1/recaps/search with a whitespace term answers the handler's envelope @contract", async ({
		api,
	}) => {
		// api/fetch.rs:243-254 — `query.term.trim()` empty becomes
		// (BAD_REQUEST, Json(ErrorResponse{…})). Present-but-blank is the shape
		// a UI sends when a user hits enter on an empty search box, so it must
		// not reach `search_recaps_by_term` with an empty LIKE pattern.
		const response = await api.get("/v1/recaps/search?term=%20%20%20");
		const body = await expectJsonStatus(response, 400, errorSchema);
		expect(body.error).toBe("term parameter is required");
	});

	test("GET /v1/recaps/search with a non-numeric limit is rejected @contract", async ({
		api,
	}) => {
		// `limit: Option<i32>` — "abc" is not a missing value, it is an
		// unparseable one, and serde_urlencoded fails the whole struct. Pinned
		// because the tempting `Option<String>` + `parse().unwrap_or(50)`
		// refactor would silently swallow a client's typo'd pagination.
		await expectStatus(await api.get("/v1/recaps/search?term=ai&limit=abc"), 400);
	});

	test("GET /v1/recaps/search for a term nothing indexes answers 200 with no results @contract", async ({
		api,
		seed,
	}) => {
		// Parallel-safe by construction: the term is derived from the run id and
		// the worker index, so nothing in `top_terms` can ever match it — which
		// makes "exactly zero results" an assertion rather than a race with
		// whatever the pipeline project just wrote.
		//
		// The distinction being pinned is empty-versus-missing: a miss on this
		// endpoint is a 200 with `{"results": []}`, unlike the windowed reads,
		// which 404. search-indexer walks this surface on a schedule and would
		// treat a 404 as an outage.
		const body = await expectJsonStatus(
			await api.get(`/v1/recaps/search?term=${encodeURIComponent(seed.absentTerm)}`),
			200,
			searchRecapsSchema,
		);
		expect(body.results).toEqual([]);
	});
});

test.describe("path-parameter validation", () => {
	test("GET /v1/morning/letters/{bad-date} answers 400 naming the format @contract", async ({
		api,
	}) => {
		// api/fetch.rs:917-928. The date is parsed by the *handler*, not by a
		// typed extractor, so the envelope is recap-worker's own — and the
		// message quotes the offending value back, which is what makes a
		// timezone-confused client debuggable.
		//
		// This also proves axum's static-versus-dynamic route precedence still
		// holds: `/v1/morning/letters/latest` and
		// `/v1/morning/letters/{target_date}` are both registered
		// (api.rs:44-51), and a router that started matching `latest` as a date
		// would answer this 400 for `latest` too.
		const body = await expectJsonStatus(
			await api.get("/v1/morning/letters/not-a-date"),
			400,
			errorSchema,
		);
		expect(body.error).toBe("Invalid date format: 'not-a-date'. Expected YYYY-MM-DD");
	});

	test("GET /v1/morning/letters/{well-formed absent date} answers 404 @contract", async ({
		api,
	}) => {
		// The other half of the pair: a parseable date with no row is a miss,
		// not a validation failure. 2020-01-01 predates the service, so this is
		// order-independent and can live outside the cold-start project.
		const body = await expectJsonStatus(
			await api.get("/v1/morning/letters/2020-01-01"),
			404,
			errorSchema,
		);
		expect(body.error).toBe("No morning letter found for date: 2020-01-01");
	});

	test("GET /v1/morning/letters/{bad-uuid}/sources answers 400 @contract", async ({ api }) => {
		// api/fetch.rs:1023-1034. Again a handler-parsed parameter, so the
		// envelope is `{"error": …}` rather than axum's plain text — unlike the
		// evaluation route below, which uses `Path<Uuid>`. The two live next to
		// each other on purpose: they are the same kind of parameter validated
		// at two different layers, and a client cannot tell without being told.
		const body = await expectJsonStatus(
			await api.get("/v1/morning/letters/not-a-uuid/sources"),
			400,
			errorSchema,
		);
		expect(body.error).toBe("Invalid letter_id: 'not-a-uuid'. Expected UUID");
	});

	test("GET /v1/morning/letters/{absent uuid}/sources answers 200 with an empty list @contract", async ({
		api,
		seed,
	}) => {
		// `get_morning_letter_sources` has no not-found branch: an unknown
		// letter and a letter with no provenance rows are the same answer
		// (api/fetch.rs:1039-1053). That is a real design decision — provenance
		// is an append-only side table, so absence is not an error — and pinning
		// it stops someone "improving" it into a 404 that breaks the SPA's
		// footnote rendering.
		const body = await expectJsonStatus(
			await api.get(`/v1/morning/letters/${seed.absentId}/sources`),
			200,
			morningLetterSourcesSchema,
		);
		expect(body).toEqual([]);
	});

	test("GET /v1/evaluation/genres/{bad-uuid} is rejected by the extractor @contract", async ({
		api,
	}) => {
		// `Path<uuid::Uuid>` (api/evaluation.rs:1067), so this never reaches the
		// handler and the body is axum's plain text — the counterpart to the
		// hand-parsed letter_id above.
		//
		// It also proves the static/dynamic precedence for this pair:
		// `/v1/evaluation/genres/latest` is registered alongside
		// `/v1/evaluation/genres/{run_id}` (api.rs:52-60), and "latest" is not a
		// UUID, so a router that resolved it to the dynamic route would answer
		// 400 for the endpoint the dashboard actually calls.
		await expectStatus(await api.get("/v1/evaluation/genres/not-a-uuid"), 400);
	});

	test("GET /v1/evaluation/genres/{absent uuid} answers 404 echoing the run_id @contract", async ({
		api,
		seed,
	}) => {
		// api/evaluation.rs:1079-1085. The echoed `run_id` is what lets a
		// dashboard correlate the miss with the run it asked for, so the
		// envelope carries two fields and both are asserted.
		const body = await expectJsonStatus(
			await api.get(`/v1/evaluation/genres/${seed.absentId}`),
			404,
			evaluationNotFoundSchema,
		);
		expect(body.error).toBe("Evaluation result not found");
		expect(body.run_id).toBe(seed.absentId);
	});

	test("GET /v1/pulse/latest?date={bad} answers 400 naming the format @contract", async ({
		api,
	}) => {
		// api/pulse.rs:117-127.
		const body = await expectJsonStatus(
			await api.get("/v1/pulse/latest?date=nope"),
			400,
			errorSchema,
		);
		expect(body.error).toBe("Invalid date format: nope. Expected YYYY-MM-DD");
	});

	test("GET /v1/pulse/latest?date={absent} answers 404 naming the date @contract", async ({
		api,
	}) => {
		// The dated miss interpolates the requested date rather than "today"
		// (api/pulse.rs:146-163) — the branch tests/cold-start.spec.ts cannot
		// reach, and the one a client hits when a user scrolls back a week.
		const body = await expectJsonStatus(
			await api.get("/v1/pulse/latest?date=2020-01-01"),
			404,
			errorSchema,
		);
		expect(body.error).toBe("No evening pulse found for date 2020-01-01");
	});
});

test.describe("admin surfaces", () => {
	test("POST /v1/evaluation/genres rejects an unreadable dataset with a hint @contract", async ({
		api,
		seed,
	}) => {
		// New coverage. `determine_dataset_path` takes `data_path` from the body
		// and falls back to `RECAP_GOLDEN_DATASET_PATH` then
		// `/app/data/golden_classification.json` (api/evaluation.rs:507-513);
		// `load_and_validate_dataset` turns any load failure into a 400 carrying
		// the path it tried and a hint (api/evaluation.rs:838-852).
		//
		// Pointing it at a per-test path that cannot exist exercises the
		// validation branch without running the classifier over the real golden
		// dataset — which is minutes of CPU and would need the model cache
		// warmed. The three-field envelope is the contract: an operator
		// debugging a mounted-volume mistake needs the resolved path back, and a
		// bare `{"error": …}` regression would take that away.
		const body = await expectJsonStatus(
			await api.post("/v1/evaluation/genres", {
				data: { data_path: `/nonexistent/${seed.token}/golden.json` },
			}),
			400,
			datasetErrorSchema,
		);
		expect(body.path).toBe(`/nonexistent/${seed.token}/golden.json`);
		expect(body.error).toContain("Failed to load golden dataset");
	});

	test("POST /admin/genre-learning rejects a payload with no thresholds @contract", async ({
		api,
	}) => {
		// New coverage. api/learning.rs:118-129. recap-subworker posts here at
		// the end of a learning run; a payload whose `graph_override` and
		// `summary` references are both absent leaves `config_payload` empty,
		// and the handler refuses rather than writing an empty config row.
		//
		// That refusal is CLAUDE.md rule 8 in miniature: the alternative — a 200
		// with `config_saved: true` over an empty JSON object — would make
		// "subworker sent nothing" indistinguishable from "subworker sent
		// values", and the genre classifier would quietly run on defaults
		// forever.
		const body = await expectJsonStatus(
			await api.post("/admin/genre-learning", { data: genreLearningBody(null) }),
			400,
			genreLearningSchema,
		);
		expect(body.status).toBe("error");
		expect(body.config_saved).toBe(false);
		expect(body.message).toBe("no configuration values provided");
	});

	// The *positive* branch of /admin/genre-learning is deliberately not here.
	// A successful call writes a `graph_override` row into `recap_worker_config`,
	// and `PipelineOrchestrator::prepare_pipeline` re-reads the latest such row
	// **before every run** and pushes it into the genre stage
	// (pipeline/orchestrator.rs:429-441, pipeline/graph_override.rs:24-66). So
	// the write is not invisible: it retunes the classifier for whatever the
	// pipeline project runs next. It lives at the end of
	// tests/pipeline.spec.ts's serial block instead, with the rest of the
	// globally-visible mutations.

	test("POST /v1/morning/letters/regenerate is mounted @contract", async ({ api }) => {
		// The regenerate endpoint runs the morning pipeline *synchronously* in
		// the request (api/fetch.rs:972-996), so driving its happy path here
		// would mean a multi-minute test whose outcome depends on the stub's
		// morning-letter payload and on a recap already existing. What is worth
		// asserting cheaply is that it exists at all: a wrong-Content-Type POST
		// is rejected by the `Json<RegenerateLatestRequest>` extractor with 415,
		// before the handler — and therefore the pipeline — runs.
		//
		// A 404 here means the `.route(...)` line at api.rs:47-50 is gone and
		// the SPA's "regenerate" button silently does nothing.
		const response = await api.post("/v1/morning/letters/regenerate", {
			headers: { "Content-Type": "text/plain" },
			data: "{}",
		});
		await expectRouteMounted(response, [415]);
	});

	test("POST /admin/jobs/retry is mounted and answers only with method POST @contract", async ({
		api,
	}) => {
		// The behavioural assertions on this endpoint need the pipeline lock and
		// live in tests/pipeline.spec.ts. This is the wiring half, which is
		// parallel-safe: GET must be 405 rather than 404, proving the path
		// resolves (api.rs:26).
		const response = await api.get("/admin/jobs/retry");
		await expectStatus(response, 405);
		expectHeaderContains(response, "Allow", "POST");
	});
});
