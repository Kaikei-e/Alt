import { test, expect, recapBody } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { uuid } from "../../_shared/ids.js";
import {
	batchRecapResponseSchema,
	fastapiErrorSchema,
	recapSummaryResponseSchema,
} from "../src/schemas.js";

/**
 * `POST /v1/summary/generate` and `/v1/summary/generate/batch` — the port of
 * `05-recap-summary.hurl` and `06-recap-batch.hurl`.
 *
 * recap-worker's structured-output path. The usecase posts to `/api/chat`
 * with a `format` JSON schema and unwraps the model's JSON string back into a
 * `RecapSummary`, so what is really under test is the round trip through
 * Ollama Structured Outputs and `_parse_summary_json`.
 *
 * The Hurl scenarios pinned `job_id` to a literal UUID typed into both the
 * fixture and the assertion. Here each test mints its own, so "the response
 * belongs to my request" is a claim the service has to earn — which matters
 * more for batch than anywhere else in the suite, since that is the one
 * endpoint that fans out and re-collects.
 */
test.describe("recap summary", () => {
	test("generates a Japanese recap for one genre @smoke @contract", async ({ api, seed }) => {
		const response = await api.post("/v1/summary/generate", {
			data: recapBody(seed.jobId, "ai"),
		});
		const body = await expectJsonStatus(response, 200, recapSummaryResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		expect(body.job_id).toBe(seed.jobId);
		expect(body.genre).toBe("ai");

		// `summary_length_bullets` is computed from the parsed bullets, so it
		// must agree with what came back. A mismatch means the metadata was
		// assembled from a different object than the summary — which is exactly
		// what a partially-applied refactor of the map/reduce path produces, and
		// what recap-worker's downstream quality gate keys off.
		expect(body.metadata.summary_length_bullets).toBe(body.summary.bullets.length);

		// `is_degraded` false is the assertion that the structured-output path
		// actually ran. Every failure mode in `recap_summary_usecase` — a JSON
		// parse failure, a validation error, a reduce that lost everything —
		// answers 200 with a degraded payload rather than an error status, so
		// status alone cannot tell a working pipeline from a broken one.
		expect(body.metadata.is_degraded).toBe(false);
	});

	test("batch preserves request order and reports no errors @contract", async ({ api }) => {
		const jobs = [
			{ jobId: uuid(), genre: "ai" },
			{ jobId: uuid(), genre: "security" },
		] as const;

		const response = await api.post("/v1/summary/generate/batch", {
			data: { requests: jobs.map((job) => recapBody(job.jobId, job.genre)) },
		});
		const body = await expectJsonStatus(response, 200, batchRecapResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		expect(body.responses).toHaveLength(jobs.length);

		// Order is a contract, not an accident: `generate_batch_summary` runs the
		// requests concurrently under a `TaskGroup` and then re-zips the results
		// against the *original* request list
		// (usecase/recap_summary_usecase.py:311). recap-worker relies on that —
		// it matches genres positionally. A refactor to "append as completed"
		// would silently shuffle genres between jobs, and only an index-wise
		// assertion catches it; comparing sets would not.
		for (const [index, job] of jobs.entries()) {
			const entry = body.responses[index];
			expect(entry, `responses[${index}] missing`).toBeDefined();
			expect(entry?.job_id).toBe(job.jobId);
			expect(entry?.genre).toBe(job.genre);
			expect(entry?.metadata.is_degraded).toBe(false);
		}

		// `errors[]` empty means every sub-request took the success path.
		// `_generate_summary_with_error_handling` swallows per-request failures
		// into this array under a 200, so an empty `errors` is the only place a
		// half-failed batch is visible.
		expect(body.errors).toEqual([]);
	});

	test("rejects a job_id that is not a UUID @contract", async ({ api }) => {
		// New coverage. `job_id` is `ApiUUID`, whose `BeforeValidator` calls
		// `UUID(value)` and lets the ValueError become a 422
		// (domain/models.py:11-21). It is recap-worker's correlation key, so a
		// coerced or accepted junk value would produce a summary nothing could
		// be matched back to.
		const response = await api.post("/v1/summary/generate", {
			data: { ...recapBody(uuid(), "ai"), job_id: "not-a-uuid" },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects an empty clusters list @contract", async ({ api, seed }) => {
		// New coverage. `clusters` is `min_length=1, max_length=300`. Without the
		// lower bound the usecase would prompt the model with no evidence and
		// return a confidently invented recap — the worst possible failure for a
		// summarization service, and one that no downstream check would flag.
		const response = await api.post("/v1/summary/generate", {
			data: { ...recapBody(seed.jobId, "ai"), clusters: [] },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects max_bullets above the ceiling @contract", async ({ api, seed }) => {
		// New coverage. `RecapSummaryOptions.max_bullets` is `ge=1, le=15`, and
		// `RecapSummary.bullets` is `max_length=15` on the way back out. A caller
		// asking for 99 must be refused at the boundary rather than have the
		// response fail validation *after* the GPU time was already spent.
		const response = await api.post("/v1/summary/generate", {
			data: { ...recapBody(seed.jobId, "ai"), options: { max_bullets: 99 } },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a batch with no requests @contract", async ({ api }) => {
		// New coverage. `BatchRecapSummaryRequest.requests` is
		// `min_length=1, max_length=50`; an empty batch would answer 200 with two
		// empty arrays and look indistinguishable from "everything failed".
		const response = await api.post("/v1/summary/generate/batch", { data: { requests: [] } });
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("both recap routes are mounted, not just the singular one @contract", async ({ api }) => {
		// 405 proves the path resolved and only the verb was wrong; a 404 would
		// mean `create_recap_summary_router` never made it into `app.include_router`
		// — the DI-wiring failure CLAUDE.md rule 8 is about, and one that a
		// "not 2xx" assertion would happily accept.
		await expectStatus(await api.get("/v1/summary/generate"), 405);
		await expectStatus(await api.get("/v1/summary/generate/batch"), 405);
	});
});
