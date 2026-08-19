import { z } from "zod";

/**
 * Response envelopes, as Zod schemas.
 *
 * The Hurl suite this replaces asserted one JSONPath at a time — `jsonpath
 * "$.summary" isString`, `jsonpath "$.model" matches ".+"` — which never says
 * anything about the fields it did not name. A response that silently dropped
 * `prompt_tokens`, or grew an `error` key, satisfied every one of those checks.
 * A schema asserts the whole envelope in one step.
 *
 * Every object schema is `passthrough()`: these are contracts on the fields
 * news-creator promises its callers (pre-processor, recap-worker,
 * rag-orchestrator, alt-backend's Morning Letter usecase), not a freeze on the
 * ones it may add. The one deliberate exception is `healthSchema`, which uses
 * a refinement to insist `models` is *absent* — see below.
 */

export { fastapiErrorSchema } from "../../_shared/schemas.js";
import { uuidSchema } from "../../_shared/schemas.js";

/** Re-exported so specs keep one import for "shapes news-creator answers with". */
export { uuidSchema };

// ---------------------------------------------------------------------------
// /health, /health/deep, /queue/status  — handler/health_handler.py
// ---------------------------------------------------------------------------

/**
 * `{status, service}` — cheap liveness, and **no `models[]`**.
 *
 * `/health` is what compose's healthcheck probes, so it must not fan out to
 * Ollama: a liveness path that does upstream I/O restart-loops the container
 * whenever the LLM hiccups (docs/runbooks/health-deep-contract.md). Upstream
 * reachability moved to `/health/deep`. `models` is therefore asserted
 * *absent* rather than merely unrequired — its return would be the regression
 * the split exists to prevent, and `passthrough()` would otherwise let it
 * through unnoticed.
 */
export const healthSchema = z
	.object({
		status: z.literal("healthy"),
		service: z.literal("news-creator"),
	})
	.passthrough()
	.refine((body) => !("models" in body), {
		message:
			"`/health` carried `models[]`, so the compose probe path is calling " +
			"list_models() again — that belongs to `/health/deep`",
	});

/** One row of `/health/deep`'s `checks[]` (infra/health_deep.py `CheckResult`). */
export const deepCheckSchema = z
	.object({
		name: z.string().min(1),
		status: z.enum(["pass", "warn", "fail"]),
		critical: z.boolean(),
		latency_ms: z.number().int().nonnegative(),
		reason: z.enum(["timeout", "unavailable", "not_ready"]).optional(),
	})
	.passthrough();

/**
 * `/health/deep` — `DeepHealthRunner.run()` rendered by `Report.as_dict()`.
 *
 * `status` is the fold over `checks[]`: a failing *critical* check makes the
 * report `fail` and the response 503, which is what keeps a dependency outage
 * out of the liveness path while still being observable.
 */
export const deepHealthSchema = z
	.object({
		status: z.enum(["pass", "warn", "fail"]),
		service: z.literal("news-creator"),
		checks: z.array(deepCheckSchema).min(1),
		latency_ms: z.number().int().nonnegative(),
		cached: z.boolean(),
	})
	.passthrough();

/**
 * `HybridPrioritySemaphore.queue_status()` (gateway/hybrid_priority_semaphore.py:846).
 *
 * `acquired_slots` is in the shape but was never asserted by the Hurl suite;
 * it is the counter the class's own leak detector is built on, and it is what
 * makes the "no slot leaked" invariant in tests/queue.spec.ts expressible.
 */
export const queueStatusSchema = z
	.object({
		rt_queue: z.number().int().nonnegative(),
		be_queue: z.number().int().nonnegative(),
		total_slots: z.number().int().min(1),
		available_slots: z.number().int().nonnegative(),
		accepting: z.boolean(),
		max_queue_depth: z.number().int().nonnegative(),
		acquired_slots: z.number().int().nonnegative(),
	})
	.passthrough();

export type QueueStatus = z.infer<typeof queueStatusSchema>;

// ---------------------------------------------------------------------------
// /api/v1/summarize — domain/models.py SummarizeResponse
// ---------------------------------------------------------------------------

/**
 * `SummarizeResponse` is a strict+frozen Pydantic model, so the wire shape is
 * exactly its fields. The token counters are `int | None`: the usecase fills
 * them from the LLM's `prompt_eval_count` / `eval_count`, which real Ollama
 * omits on some paths (usecase/summarize_usecase.py:379).
 */
export const summarizeResponseSchema = z
	.object({
		success: z.literal(true),
		article_id: z.string().min(1),
		summary: z.string().min(1),
		model: z.string().min(1),
		prompt_tokens: z.number().int().nonnegative().nullable().optional(),
		completion_tokens: z.number().int().nonnegative().nullable().optional(),
		total_duration_ms: z.number().nonnegative().nullable().optional(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// /api/generate — handler/generate_handler.py response_dict
// ---------------------------------------------------------------------------

/**
 * The Ollama-shaped pass-through envelope the handler assembles by hand.
 *
 * `done` and `done_reason` are always present (the handler defaults them to
 * `True` / `"stop"`); the three counters are appended only when the upstream
 * supplied them, hence `.optional()`. Pinning `done_reason: "stop"` matches
 * the Hurl assertion and is what a caller keys "the model finished" off.
 */
export const generateResponseSchema = z
	.object({
		model: z.string().min(1),
		response: z.string().min(1),
		done: z.literal(true),
		done_reason: z.literal("stop"),
		prompt_eval_count: z.number().int().nonnegative().optional(),
		eval_count: z.number().int().nonnegative().optional(),
		total_duration: z.number().int().nonnegative().optional(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// /v1/summary/generate(/batch) — domain/models.py RecapSummary*
// ---------------------------------------------------------------------------

/** `RecapSummaryMetadata`; `language` on the summary is `pattern="^ja$"`. */
export const recapMetadataSchema = z
	.object({
		model: z.string().min(1),
		temperature: z.number().nullable().optional(),
		prompt_tokens: z.number().int().nonnegative().nullable().optional(),
		completion_tokens: z.number().int().nonnegative().nullable().optional(),
		processing_time_ms: z.number().int().nonnegative().nullable().optional(),
		json_validation_errors: z.number().int().nonnegative(),
		summary_length_bullets: z.number().int().min(1),
		is_degraded: z.boolean(),
		degradation_reason: z.string().nullable().optional(),
	})
	.passthrough();

export const recapSummarySchema = z
	.object({
		title: z.string().min(1).max(200),
		bullets: z.array(z.string().min(1)).min(1).max(15),
		language: z.literal("ja"),
		references: z
			.array(
				z
					.object({
						id: z.number().int().min(1),
						url: z.string().min(1),
						domain: z.string().min(1),
					})
					.passthrough(),
			)
			.nullable()
			.optional(),
	})
	.passthrough();

export const recapSummaryResponseSchema = z
	.object({
		job_id: uuidSchema,
		genre: z.string().min(1),
		summary: recapSummarySchema,
		metadata: recapMetadataSchema,
	})
	.passthrough();

export const batchRecapResponseSchema = z
	.object({
		responses: z.array(recapSummaryResponseSchema),
		errors: z.array(
			z
				.object({
					job_id: uuidSchema,
					genre: z.string(),
					error: z.string(),
				})
				.passthrough(),
		),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// /api/v1/expand-query, /api/v1/plan-query — the rag-orchestrator surface
// ---------------------------------------------------------------------------

export const expandQueryResponseSchema = z
	.object({
		expanded_queries: z.array(z.string().min(1)).min(1),
		original_query: z.string().min(1),
		model: z.string().min(1),
		processing_time_ms: z.number().nonnegative().nullable().optional(),
	})
	.passthrough();

/**
 * `QueryPlan`. The three enum-ish fields are declared as free strings in
 * Pydantic (their allowed values live only in the field *description*, because
 * a small model has to be coaxed rather than constrained), so the schema pins
 * membership here — that is exactly the check the model can fail and the
 * type system cannot.
 */
export const queryPlanSchema = z
	.object({
		reasoning: z.string().min(1),
		resolved_query: z.string().min(1),
		search_queries: z.array(z.string().min(1)).min(1),
		intent: z.enum([
			"causal_explanation",
			"temporal",
			"synthesis",
			"comparison",
			"fact_check",
			"topic_deep_dive",
			"general",
		]),
		retrieval_policy: z.enum(["global_only", "article_only"]),
		answer_format: z.enum(["causal_analysis", "summary", "list", "detail", "comparison"]),
		should_clarify: z.boolean(),
		topic_entities: z.array(z.string()),
	})
	.passthrough();

export const planQueryResponseSchema = z
	.object({
		plan: queryPlanSchema,
		original_query: z.string().min(1),
		model: z.string().min(1),
		processing_time_ms: z.number().nonnegative().nullable().optional(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// /api/chat — the verbatim Ollama proxy (handler/chat_handler.py)
// ---------------------------------------------------------------------------

/**
 * The non-streaming chat envelope. `chat_endpoint` returns
 * `JSONResponse(content=result)` — whatever the upstream said, unmodified — so
 * this is a contract on Ollama's shape as much as on news-creator's.
 */
export const chatResponseSchema = z
	.object({
		model: z.string().min(1),
		message: z
			.object({
				role: z.literal("assistant"),
				content: z.string().min(1),
			})
			.passthrough(),
		done: z.literal(true),
	})
	.passthrough();

/** One NDJSON line of the streaming chat proxy. */
export const chatStreamChunkSchema = z
	.object({
		model: z.string().min(1),
		message: z
			.object({
				role: z.literal("assistant"),
				content: z.string(),
			})
			.passthrough(),
		done: z.boolean(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// /v1/morning-letter/generate — domain/models.py MorningLetter*
// ---------------------------------------------------------------------------

/**
 * `MorningLetterSection.key` carries its grammar as a Pydantic `pattern`, so
 * a section whose key drifts is rejected server-side. Repeating the pattern
 * here is what proves the server actually enforced it rather than answering a
 * hand-built dict that bypassed the model.
 */
export const morningLetterSectionSchema = z
	.object({
		key: z.string().regex(/^(lead|top3|what_changed|by_genre:[a-z0-9_-]+)$/),
		title: z.string().min(1),
		bullets: z.array(z.string().min(1)).min(1),
		genre: z.string().nullable().optional(),
		narrative: z.string().nullable().optional(),
	})
	.passthrough();

export const morningLetterContentSchema = z
	.object({
		schema_version: z.literal(1),
		lead: z.string().min(1),
		sections: z.array(morningLetterSectionSchema).min(1),
		generated_at: z.string().min(1),
		source_recap_window_days: z.number().int().nullable().optional(),
	})
	.passthrough();

export const morningLetterResponseSchema = z
	.object({
		target_date: z.string().min(1),
		edition_timezone: z.string().min(1),
		content: morningLetterContentSchema,
		metadata: recapMetadataSchema,
	})
	.passthrough();

// ---------------------------------------------------------------------------
// OpenAPI — used as the route-registration probe (see tests/routing.spec.ts)
// ---------------------------------------------------------------------------

/**
 * Just enough of the OpenAPI 3.1 document to enumerate what FastAPI actually
 * mounted. `paths` maps a path template to a map of lower-case HTTP methods.
 */
export const openApiSchema = z
	.object({
		openapi: z.string().regex(/^3\./),
		info: z.object({ title: z.string(), version: z.string() }).passthrough(),
		paths: z.record(z.record(z.unknown())),
	})
	.passthrough();
