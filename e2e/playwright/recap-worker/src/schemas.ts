import { z } from "zod";
import { timestampSchema, uuidSchema } from "../../_shared/schemas.js";

/**
 * The response shapes recap-worker promises.
 *
 * The Hurl suite this replaces asserted six windowed fields with
 * `jsonpath "$.window_start" exists` — an assertion that passes for `null`,
 * `""` and `{}` alike. Every one of those is a real timestamp here, so a
 * handler that started emitting `null` for a missing `window_end` (the
 * `unwrap_or_default()` path in `get_recap_by_window`) would have kept the
 * Hurl suite green. Schemas make that a failure.
 *
 * Everything is `.passthrough()` by convention: these are contracts on the
 * fields the service promises, not a freeze on the ones it may add. The one
 * `.strict()` is on the live-probe envelope, where an extra field *is* the
 * regression — see `healthLiveSchema`.
 *
 * All of recap-worker's own handlers are axum + serde JSON in **snake_case**
 * (`#[serde(rename_all = "snake_case")]` on `HealthReport`, plain field names
 * everywhere else). The camelCase Connect-RPC wire format in this stack
 * belongs to its *upstream* (alt-data-hub), not to anything asserted here.
 */

// ---------------------------------------------------------------------------
// Health (api/health.rs)
// ---------------------------------------------------------------------------

/**
 * `GET /health/live`.
 *
 * `.strict()` on purpose. `HealthReport` carries an
 * `#[serde(skip_serializing_if = "Option::is_none")] detail` field that is
 * populated *only* by `HealthReport::degraded` (api/health.rs:24-28). A live
 * response that grew a `detail` key would mean the dependency-free probe had
 * started reporting on a dependency — which is precisely what makes
 * `/health/live` unusable as the compose gate it is used as.
 */
export const healthLiveSchema = z.object({ status: z.literal("live") }).strict();

/** `GET /health/ready` on a stack whose subworker + news-creator both answer. */
export const healthReadySchema = z.object({ status: z.literal("ready") }).strict();

/**
 * The 503 body when either upstream ping fails (api/health.rs:36-48).
 *
 * Not asserted on a healthy stack — it is here so a spec that ever needs to
 * discriminate "degraded" from "some other 503" has the shape to hand.
 */
export const healthDegradedSchema = z
	.object({ status: z.literal("degraded"), detail: z.string().min(1) })
	.strict();

// ---------------------------------------------------------------------------
// Error envelopes
// ---------------------------------------------------------------------------

/**
 * recap-worker's own error envelope: `struct ErrorResponse { error: String }`,
 * repeated in api/fetch.rs, api/generate.rs, api/admin.rs and api/pulse.rs.
 *
 * `min(1)` because the Hurl suite compared the string exactly and an empty
 * string would satisfy a bare `z.string()`.
 */
export const errorSchema = z.object({ error: z.string().min(1) }).passthrough();

/** `POST /v1/evaluation/genres` dataset-load failure (api/evaluation.rs:843-852). */
export const datasetErrorSchema = z
	.object({
		error: z.string().min(1),
		path: z.string().min(1),
		hint: z.string().min(1),
	})
	.passthrough();

/** `GET /v1/evaluation/genres/{run_id}` miss (api/evaluation.rs:1079-1085). */
export const evaluationNotFoundSchema = z
	.object({ error: z.string().min(1), run_id: uuidSchema })
	.passthrough();

// ---------------------------------------------------------------------------
// Trigger (api/generate.rs)
// ---------------------------------------------------------------------------

/**
 * `POST /v1/generate/recaps/{7,3}days` → 202.
 *
 * `genres` is the *normalized* list (`normalize_genres`: trim, lowercase,
 * dedupe, drop empties) and is never empty — an empty result short-circuits to
 * the 400 above. `nonEmptyArray` is what states that; the Hurl suite only
 * checked that the list contained `"ai"`, which an
 * `["ai", "", "  ", "AI"]` regression would also have satisfied.
 */
export const triggerAcceptedSchema = z
	.object({
		job_id: uuidSchema,
		genres: z.array(z.string().min(1)).min(1),
		status: z.literal("accepted"),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Recap read model (api/fetch.rs)
// ---------------------------------------------------------------------------

export const evidenceLinkSchema = z
	.object({
		article_id: z.string().min(1),
		title: z.string(),
		source_url: z.string().min(1),
		published_at: timestampSchema,
		lang: z.string().min(1),
	})
	.passthrough();

export const referenceSchema = z
	.object({
		id: z.number().int(),
		url: z.string().min(1),
		domain: z.string().min(1),
		article_id: z.string().optional(),
	})
	.passthrough();

export const recapGenreSchema = z
	.object({
		genre: z.string().min(1),
		summary: z.string(),
		top_terms: z.array(z.string()),
		article_count: z.number().int(),
		cluster_count: z.number().int(),
		// May legitimately be empty: `evidence_links` is built from the
		// clusters' `evidence` vectors and a cluster with no evidence rows
		// contributes nothing. The *genre list* is what must not be empty.
		evidence_links: z.array(evidenceLinkSchema),
		bullets: z.array(z.string()),
		references: z.array(referenceSchema).optional(),
	})
	.passthrough();

/**
 * `GET /v1/recaps/{7,3}days` → 200.
 *
 * Every one of `job_id`, `executed_at`, `window_start`, `window_end` was an
 * `exists` check in the Hurl suite. They are a UUID and three RFC 3339
 * instants (`to_rfc3339()` on chrono values in `get_recap_by_window`), so
 * that is what is asserted.
 */
export const recapSummarySchema = z
	.object({
		job_id: uuidSchema,
		executed_at: timestampSchema,
		window_start: timestampSchema,
		window_end: timestampSchema,
		total_articles: z.number().int(),
		genres: z.array(recapGenreSchema).min(1),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Search + indexable genres (api/fetch.rs) — the search-indexer contract
// ---------------------------------------------------------------------------

export const recapSearchHitSchema = z
	.object({
		job_id: uuidSchema,
		executed_at: timestampSchema,
		window_days: z.number().int(),
		genre: z.string().min(1),
		summary: z.string(),
		top_terms: z.array(z.string()),
		tags: z.array(z.string()),
		bullets: z.array(z.string()),
	})
	.passthrough();

export const searchRecapsSchema = z
	.object({ results: z.array(recapSearchHitSchema) })
	.passthrough();

/**
 * `GET /v1/recaps/genres/indexable`.
 *
 * `has_more` is computed as `hits.len() == limit` — a page-boundary flag, not
 * a cursor. Asserting it is a boolean *and* asserting the relationship against
 * `results.length` (tests/read-surface.spec.ts) is what would catch the
 * classic off-by-one that makes search-indexer either loop forever or stop
 * one page early.
 */
export const indexableGenresSchema = z
	.object({ results: z.array(recapSearchHitSchema), has_more: z.boolean() })
	.passthrough();

// ---------------------------------------------------------------------------
// Morning surface (api/fetch.rs)
// ---------------------------------------------------------------------------

export const morningArticleGroupSchema = z
	.object({
		group_id: uuidSchema,
		article_id: uuidSchema,
		is_primary: z.boolean(),
		created_at: timestampSchema,
	})
	.passthrough();

export const morningUpdatesSchema = z.array(morningArticleGroupSchema);

export const morningLetterSourceSchema = z
	.object({
		letter_id: uuidSchema,
		section_key: z.string(),
		article_id: uuidSchema,
		source_type: z.string(),
		position: z.number().int(),
	})
	.passthrough();

export const morningLetterSourcesSchema = z.array(morningLetterSourceSchema);

export const morningLetterSectionSchema = z
	.object({
		key: z.string(),
		title: z.string(),
		bullets: z.array(z.string()),
		genre: z.string().optional(),
		narrative: z.string().optional(),
		why_reasons: z
			.array(
				z
					.object({
						code: z.string(),
						ref_id: z.string().optional(),
						tag: z.string().optional(),
					})
					.passthrough(),
			)
			.optional(),
	})
	.passthrough();

/**
 * `GET /v1/morning/letters/{latest,YYYY-MM-DD}` → 200.
 *
 * `schema_version` is pinned to 1 because `parse_morning_letter_body` refuses
 * anything else with a 500 (api/fetch.rs:775-782) — so a 200 carrying any
 * other value would mean the guard stopped running.
 */
export const morningLetterSchema = z
	.object({
		id: uuidSchema,
		target_date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
		edition_timezone: z.string().min(1),
		is_degraded: z.boolean(),
		schema_version: z.literal(1),
		generation_revision: z.number().int(),
		model: z.string().nullable(),
		created_at: timestampSchema,
		etag: z.string().min(1),
		body: z
			.object({
				lead: z.string(),
				sections: z.array(morningLetterSectionSchema),
				generated_at: z.string().min(1),
				source_recap_window_days: z.number().int().optional(),
				through_line: z.string().optional(),
			})
			.passthrough(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Evening Pulse (api/pulse.rs)
// ---------------------------------------------------------------------------

export const pulseLatestSchema = z
	.object({
		job_id: z.string().min(1),
		version: z.string().min(1),
		date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
		generated_at: z.string().min(1),
		status: z.string().min(1),
		topics: z.array(z.object({ cluster_id: z.number().int() }).passthrough()),
		quiet_day: z.unknown().optional(),
		diagnostics: z.unknown().optional(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Admin (api/admin.rs, api/learning.rs)
// ---------------------------------------------------------------------------

/**
 * `POST /admin/jobs/retry` → 202.
 *
 * `retried_failed_job_id` is the whole point of the endpoint's rule-8 fix
 * (api/admin.rs:78-89): the previous implementation reported 202 whether or
 * not there was anything to retry. A 202 that cannot name the job it is
 * retrying is the no-op wearing a success status.
 */
export const retryAcceptedSchema = z
	.object({
		job_id: uuidSchema,
		retried_failed_job_id: uuidSchema,
		status: z.literal("accepted"),
	})
	.passthrough();

export const genreLearningSchema = z
	.object({
		status: z.enum(["success", "error"]),
		config_saved: z.boolean(),
		message: z.string().min(1),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Dashboard (api/dashboard.rs, store/models.rs)
// ---------------------------------------------------------------------------

/** `recap_jobs.status` / `StatusTransitionResponse.status` (store/dao/types.rs). */
export const jobStatusSchema = z.enum([
	"pending",
	"running",
	"completed",
	"failed",
	"morning_completed",
]);

export const jobStatsSchema = z
	.object({
		success_rate_24h: z.number(),
		avg_duration_secs: z.number().int().nullable(),
		total_jobs_24h: z.number().int(),
		running_jobs: z.number().int(),
		failed_jobs_24h: z.number().int(),
	})
	.passthrough();

export const systemMetricSchema = z
	.object({
		job_id: uuidSchema.nullable(),
		timestamp: timestampSchema,
		// `metrics` is a raw jsonb column; the handler does not interpret it.
		metrics: z.unknown(),
	})
	.passthrough();

export const recentActivitySchema = z
	.object({
		job_id: uuidSchema.nullable(),
		metric_type: z.string(),
		timestamp: timestampSchema,
	})
	.passthrough();

export const logErrorSchema = z
	.object({
		timestamp: timestampSchema,
		error_type: z.string(),
		error_message: z.string().nullable(),
		raw_line: z.string().nullable(),
		service: z.string().nullable(),
	})
	.passthrough();

export const adminJobSchema = z
	.object({
		job_id: uuidSchema,
		kind: z.string(),
		status: z.string(),
		started_at: timestampSchema,
		finished_at: timestampSchema.nullable(),
		error: z.string().nullable(),
	})
	.passthrough();

export const recapJobSchema = z
	.object({
		job_id: uuidSchema,
		status: z.string(),
		last_stage: z.string().nullable(),
		kicked_at: timestampSchema,
		updated_at: timestampSchema,
	})
	.passthrough();

export const statusTransitionSchema = z
	.object({
		id: z.number().int(),
		status: jobStatusSchema,
		stage: z.string().nullable(),
		transitioned_at: timestampSchema,
		reason: z.string().nullable(),
		actor: z.string(),
	})
	.passthrough();

export const recentJobSummarySchema = z
	.object({
		job_id: uuidSchema,
		status: jobStatusSchema,
		last_stage: z.string().nullable(),
		kicked_at: timestampSchema,
		updated_at: timestampSchema,
		duration_secs: z.number().int().nullable(),
		trigger_source: z.enum(["system", "user"]),
		user_id: uuidSchema.nullable(),
		status_history: z.array(statusTransitionSchema),
	})
	.passthrough();

export const activeJobInfoSchema = z
	.object({
		job_id: uuidSchema,
		status: jobStatusSchema,
		current_stage: z.string().nullable(),
		stage_index: z.number().int(),
		stages_completed: z.array(z.string()),
		genre_progress: z.record(
			z
				.object({
					status: z.enum(["pending", "running", "succeeded", "failed"]),
					cluster_count: z.number().int().nullable(),
					article_count: z.number().int().nullable(),
				})
				.passthrough(),
		),
		total_articles: z.number().int().nullable(),
		user_article_count: z.number().int().nullable(),
		kicked_at: timestampSchema,
		trigger_source: z.enum(["system", "user"]),
	})
	.passthrough();

export const userJobContextSchema = z
	.object({
		user_article_count: z.number().int(),
		user_jobs_count: z.number().int(),
		user_feed_ids: z.array(uuidSchema),
	})
	.passthrough();

/**
 * `GET /v1/dashboard/job-progress` → 200.
 *
 * `active_job` is `null` unless a pipeline is in flight, and `user_context` is
 * `null` unless `user_id` was supplied — so both are nullable here and the
 * specs assert the *conditional* presence, which is the actual contract.
 */
export const jobProgressSchema = z
	.object({
		active_job: activeJobInfoSchema.nullable(),
		recent_jobs: z.array(recentJobSummarySchema),
		stats: jobStatsSchema,
		user_context: userJobContextSchema.nullable(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Evaluation (api/evaluation.rs)
// ---------------------------------------------------------------------------

export const evaluationResultSchema = z
	.object({
		run: z.object({ run_id: uuidSchema }).passthrough(),
		metrics: z.array(z.unknown()),
	})
	.passthrough();
