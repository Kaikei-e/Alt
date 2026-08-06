import { z } from "zod";

/**
 * Response envelopes, as Zod schemas.
 *
 * The Hurl suite this replaces could only assert one JSONPath at a time, so
 * `/api/v1/extract-tags` was pinned as five independent facts —
 * `$.success == true`, `$.tags isCollection`, `$.confidence isNumber`, … —
 * none of which noticed a field disappearing or a new one arriving in the
 * wrong shape. A schema asserts the whole envelope in one step, and the
 * `refine` clauses below go further: they pin the *relationships between*
 * fields that the extractor guarantees, which no per-JSONPath assertion can
 * express at all.
 *
 * Every object schema is `passthrough()` by convention: a contract on the
 * fields tag-generator promises, not a freeze on the ones it may add.
 */

/** Primitives shared with the rest of the fleet, re-exported for one import. */
export { uuidSchema } from "../../_shared/schemas.js";
export { connectErrorSchema } from "../../_shared/connect.js";
import { uuidSchema } from "../../_shared/schemas.js";

/**
 * `GET /health` (auth_service.py:487-508).
 *
 * FastAPI's default encoder, so snake_case-by-nature but in fact two flat
 * string fields; nothing proto3-JSON here. `service` is pinned as a literal
 * because it is how an operator tells a misrouted probe (nginx pointing at
 * news-creator, say) from a genuinely healthy tag-generator.
 *
 * A 200 here is a stronger statement than it looks: the handler raises 503
 * when the background batch service has died or when any stream consumer is
 * marked unhealthy, so this doubles as the liveness assertion for the
 * `redis-streams-consumer` / `redis-streams-tags-consumer` threads.
 */
export const healthSchema = z
	.object({
		status: z.literal("healthy"),
		service: z.literal("tag-generator"),
	})
	.passthrough();

/**
 * `POST /api/v1/extract-tags` success envelope (auth_service.py:563-569),
 * built from `TagExtractionOutcome` (tag_extractor/extract.py:72-85).
 *
 * The two refinements are invariants of `_compute_confidence` and of the
 * early-return branches, not guesses:
 *
 *  1. `_compute_confidence` returns 0.0 for an empty tag list and
 *     `0.7*coverage + 0.3*length_factor` otherwise, where coverage is at
 *     least `1/top_keywords` — so a non-empty tag list can never score 0.
 *     A handler that started returning tags with a hard-coded confidence of
 *     0, or confidence without tags, is a real regression this catches and
 *     `jsonpath "$.confidence" isNumber` does not.
 *
 *  2. `"und"` is not a langdetect output. It is written by exactly the two
 *     early returns in `extract_tags_with_metrics` — sanitization rejected
 *     the input, or the sanitized text was shorter than
 *     `min_text_length` (10) — and both of those also set `tags=[]`,
 *     `confidence=0.0` and `inference_ms=0.0`. So "the pipeline declined to
 *     run" must be reported consistently across all four fields; a build
 *     that reported `und` while charging inference time would mean the
 *     early-return contract had been reorganised without anyone noticing.
 */
export const extractTagsSchema = z
	.object({
		success: z.literal(true),
		tags: z.array(z.string()),
		confidence: z.number().min(0).max(1),
		inference_ms: z.number().min(0),
		language: z.string().min(1),
	})
	.passthrough()
	.refine((body) => (body.tags.length > 0) === (body.confidence > 0), {
		message:
			"confidence must be > 0 exactly when tags is non-empty " +
			"(tag_extractor/extract.py:_compute_confidence)",
	})
	.refine(
		(body) =>
			body.language !== "und" ||
			(body.tags.length === 0 && body.confidence === 0 && body.inference_ms === 0),
		{
			message:
				'language "und" is only written by the two early returns in ' +
				"extract_tags_with_metrics, which also zero tags/confidence/inference_ms",
		},
	);

/**
 * FastAPI + Pydantic v2 request-validation envelope.
 *
 * `_shared/schemas.ts` has a looser `fastapiErrorSchema` that also admits the
 * `{"detail": "Not Found"}` string form. This one is deliberately the array
 * form only: every scenario using it is asserting that *field-level*
 * validation ran, and a schema that also accepted a bare string would pass
 * for a generic error page.
 */
export const validationErrorSchema = z
	.object({
		detail: z
			.array(
				z
					.object({
						loc: z.array(z.union([z.string(), z.number()])),
						msg: z.string().min(1),
						type: z.string().min(1),
					})
					.passthrough(),
			)
			.min(1, "a 422 with an empty detail array tells a caller nothing"),
	})
	.passthrough();

/** Starlette's own 404 body — the control for the route-registration specs. */
export const notFoundSchema = z
	.object({ detail: z.literal("Not Found") })
	.passthrough();

/** Starlette's 405 body. */
export const methodNotAllowedSchema = z
	.object({ detail: z.literal("Method Not Allowed") })
	.passthrough();

/**
 * The OpenAPI document FastAPI generates from the decorators
 * (`FastAPI(title="Tag Generator Service", version="1.0.0")`,
 * auth_service.py:431).
 *
 * This is tag-generator's equivalent of alt-backend's Connect service
 * registration probe: the document is generated from the live route table, so
 * a decorator deleted in a refactor — or a module that failed to import and
 * left its routes unregistered — shows up here as a missing path rather than
 * as a 404 that reads like a typo in the test.
 */
export const openApiSchema = z
	.object({
		openapi: z.string().regex(/^3\./, "expected an OpenAPI 3.x document"),
		info: z
			.object({ title: z.string().min(1), version: z.string().min(1) })
			.passthrough(),
		paths: z.record(z.record(z.unknown())),
	})
	.passthrough();

/**
 * One tag inside mq-hub's `GenerateTagsForArticleResponse`.
 *
 * The `id` is minted by tag-generator as `str(uuid.uuid4())`
 * (stream_event_handler.py:_generate_tags_inline), so it is a real UUID and
 * worth pinning as one — the field is what a consumer would use to dedupe.
 *
 * `confidence` is optional because this is proto3-JSON: connect-go marshals
 * with default `protojson.MarshalOptions`, which omits zero values, and a tag
 * whose KeyBERT score rounded to 0.0 would arrive with the field absent.
 */
export const generatedTagSchema = z
	.object({
		id: uuidSchema,
		name: z.string().min(1),
		confidence: z.number().optional(),
	})
	.passthrough();

/**
 * `services.mqhub.v1.MQHubService/GenerateTagsForArticle` response.
 *
 * Every field is optional, and that is a statement about the wire rather than
 * laziness: connect-go's JSON codec uses default `protojson.MarshalOptions`,
 * so `success:false`, `inferenceMs:0`, an empty `tags` list and an empty
 * `errorMessage` are all **omitted entirely**. A schema that required
 * `success` would fail on precisely the error arm it exists to describe.
 * Tests therefore read `body.success ?? false`, and assert presence
 * explicitly where presence is the point.
 */
export const generateTagsResponseSchema = z
	.object({
		success: z.boolean().optional(),
		articleId: z.string().optional(),
		tags: z.array(generatedTagSchema).optional(),
		inferenceMs: z.number().optional(),
		errorMessage: z.string().optional(),
	})
	.passthrough();

/** mq-hub's plain-HTTP health handler (mq-hub/app/main.go:100-131). */
export const mqhubHealthSchema = z
	.object({
		healthy: z.boolean(),
		redis_status: z.string().min(1),
		uptime_seconds: z.number(),
	})
	.passthrough();
