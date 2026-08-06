import { z } from "zod";
import { timestampSchema, uuidSchema } from "../../_shared/schemas.js";

/**
 * The response shapes rag-orchestrator promises.
 *
 * The Hurl suite this replaces asserted single JSONPaths — `$.status`,
 * `$.conversations[0].title` — which is all Hurl can do. Nothing checked that
 * `conversations[0]` had a `createdAt`, or that `messageCount` was a number
 * rather than a string, or that the response was not `{"error": ...}` with a
 * coincidentally-matching field. A schema asserts the whole envelope at once,
 * so a handler that changes shape fails here.
 *
 * ## proto3 JSON zero-value omission
 *
 * Every `.optional()` on a Connect response below is the *wire format*, not
 * laxity. connect-go marshals with protojson's defaults
 * (`EmitUnpopulated: false`), so a zero-valued scalar and an empty repeated
 * field are simply absent from the JSON. `ListConversations` for a user with
 * no rows is literally the two bytes `{}` — which is exactly what
 * `05-augur-list-empty.hurl` pinned, and what tests/augur-history.spec.ts
 * still pins on the raw body in addition to this schema.
 *
 * Everything is `.passthrough()` by convention: these are contracts on the
 * fields the service promises, not a freeze on the ones it may add.
 */

// ---------------------------------------------------------------------------
// Health / readiness (Echo REST, cmd/server/main.go:125-141)
// ---------------------------------------------------------------------------

/** `GET /healthz` — the always-200 liveness handler; must not depend on DB state. */
export const healthzSchema = z.object({ status: z.literal("ok") }).passthrough();

/**
 * `GET /readyz` — 200 `{"status":"ready"}` only after `dbPool.Ping` succeeds.
 *
 * A literal rather than a union with `"db down"`: the 503 branch writes that
 * other value, so accepting both would turn "the DB pool is live" into "the
 * handler ran".
 */
export const readyzSchema = z.object({ status: z.literal("ready") }).passthrough();

/**
 * `GET /connect/health` — the static handler the Connect mux serves itself
 * (connect/server.go:76-80), independent of the Augur / MorningLetter
 * handlers. `service` is part of the contract: it is how an operator tells a
 * response from *this* listener apart from the Echo one.
 */
export const connectHealthSchema = z
	.object({ status: z.literal("healthy"), service: z.literal("connect-rpc") })
	.passthrough();

// ---------------------------------------------------------------------------
// AugurService (Connect, JSON codec)
// ---------------------------------------------------------------------------

/**
 * `alt.augur.v2.CitationKind`, as protojson spells enums: the value *name*, not
 * the numeric tag. `CITATION_KIND_UNSPECIFIED` is the zero value and is
 * therefore never on the wire — hence its absence from the enum here and the
 * `.optional()` on the field.
 */
export const citationKindSchema = z.enum([
	"CITATION_KIND_WEB",
	"CITATION_KIND_ARTICLE",
	"CITATION_KIND_SUMMARY",
]);

export const citationSchema = z
	.object({
		url: z.string().optional(),
		title: z.string().optional(),
		publishedAt: z.string().optional(),
		kind: citationKindSchema.optional(),
		refId: z.string().optional(),
	})
	.passthrough();

export const chatMessageSchema = z
	.object({
		/**
		 * Constrained to the two values the `augur_messages` CHECK constraint
		 * admits (rag-migration-atlas/migrations/20260413120000). A row that
		 * arrived with anything else would mean the constraint was dropped.
		 */
		role: z.enum(["user", "assistant"]),
		content: z.string().min(1),
		createdAt: timestampSchema.optional(),
		citations: z.array(citationSchema).optional(),
		relatedCitations: z.array(citationSchema).optional(),
	})
	.passthrough();

export const conversationSummarySchema = z
	.object({
		id: uuidSchema,
		title: z.string().min(1),
		createdAt: timestampSchema,
		/**
		 * Required, unlike most timestamps here: the view COALESCEs it to the
		 * conversation's `created_at`, so it is never zero and therefore never
		 * omitted. If it ever went missing the history list would lose its sort
		 * key and the keyset cursor would stop resolving.
		 */
		lastActivityAt: timestampSchema,
		lastMessagePreview: z.string().optional(),
		messageCount: z.number().int().nonnegative().optional(),
	})
	.passthrough();

export const listConversationsResponseSchema = z
	.object({
		conversations: z.array(conversationSummarySchema).optional(),
		nextPageToken: z.string().optional(),
	})
	.passthrough();

export const getConversationResponseSchema = z
	.object({
		id: uuidSchema,
		title: z.string().min(1),
		createdAt: timestampSchema,
		messages: z.array(chatMessageSchema).optional(),
	})
	.passthrough();

/**
 * `DeleteConversationResponse` is an empty message, so a successful delete is
 * the JSON object `{}`. `.strict()` on purpose: this is one of the few places
 * where an extra field *is* the bug — a `code` or `message` key here would mean
 * a Connect error envelope arrived under a 200, which is precisely what
 * `07-augur-delete-idempotent.hurl` guarded with `jsonpath "$.code" not exists`.
 */
export const deleteConversationResponseSchema = z.object({}).strict();

export type ListConversationsResponse = z.infer<typeof listConversationsResponseSchema>;
export type GetConversationResponse = z.infer<typeof getConversationResponseSchema>;
export type ConversationSummary = z.infer<typeof conversationSummarySchema>;

// ---------------------------------------------------------------------------
// REST (rag_http/handler.go)
// ---------------------------------------------------------------------------

/**
 * Every failure branch of the Echo handlers writes `{"error": "<literal>"}`
 * (rag_http/handler.go). Locking the message keeps the *reason* under test:
 * "missing article_id" and "invalid article_id" are two different validation
 * gates, and a schema that only said "some error" would let one swallow the
 * other.
 */
export const restErrorSchema = z.object({ error: z.string().min(1) }).passthrough();

/** `POST /internal/rag/index/delete` is a declared-but-unimplemented route. */
export const notImplementedSchema = z
	.object({ status: z.literal("not implemented") })
	.passthrough();

/**
 * `POST /internal/rag/backfill` accepted: HTTP 202 with the enqueued
 * `rag_jobs.id`. The id must be a real UUID — it is `uuid.New()` in the
 * handler, and a caller polling by it has nothing else to go on.
 */
export const backfillAcceptedSchema = z
	.object({ job_id: uuidSchema, status: z.literal("queued") })
	.passthrough();
