import { z } from "zod";
import {
	ERROR_ENVELOPE_MESSAGE,
	notAnErrorEnvelope,
	timestampSchema,
	uuidSchema,
} from "../../_shared/schemas.js";

/**
 * Response envelopes, as Zod schemas.
 *
 * The Hurl suite this replaces pinned almost nothing about response bodies —
 * its own README says so: "Response bodies are not field-pinned: Connect's
 * stock protojson codec omits zero-valued scalars, so asserting on them would
 * encode a codec detail." That reasoning is right about *scalars* and wrong
 * as a conclusion. Omission is itself a contract: `datahubOpts` in
 * connect/v2/datahub/server.go installs interceptors only — no
 * `codec.EmitUnpopulatedJSONCodec`, unlike alt-backend's operator listener —
 * so every zero-valued field is guaranteed **absent**, and every field that
 * cannot be zero is guaranteed **present**. A schema can encode both halves,
 * which is strictly more than `HTTP 200` on its own said.
 *
 * So: fields the handler can leave zero are `.optional()`; fields it always
 * writes are required. Everything is `.passthrough()` — these are contracts on
 * the fields alt-data-hub promises, not a freeze on the ones it may add.
 */

export { connectErrorSchema } from "../../_shared/connect.js";
export { timestampSchema, uuidSchema } from "../../_shared/schemas.js";

/**
 * `/health` on the **mTLS** listener.
 *
 * `connect-rpc`, not `alt-data-hub`: this is the health route every
 * Connect-RPC listener in the module mounts (connect/v2/muxutil.RegisterHealth
 * writes the literal `{"status":"healthy","service":"connect-rpc"}`), and it
 * names the surface rather than the process. The per-binary name lives on the
 * ops listener, which is why the two schemas below are different types rather
 * than one shared "health" shape — swapping them is a real mistake this
 * catches.
 */
export const connectHealthSchema = z
	.object({
		status: z.literal("healthy"),
		service: z.literal("connect-rpc"),
	})
	.passthrough();

/**
 * `/health` on the **ops** listener.
 *
 * `internal/bootstrap.NewOpsHandler` encodes `{"status","service"}` with the
 * service name it was constructed with — `alt-data-hub`, from
 * cmd/datahub/main.go's `serviceName` constant. Pinning the literal is what
 * makes this an assertion about *which binary answered*: all three split
 * binaries share :9110's shape, and in a slice where the wrong one came up on
 * that name every other assertion in this suite would still pass.
 */
export const opsHealthSchema = z
	.object({
		status: z.literal("healthy"),
		service: z.literal("alt-data-hub"),
	})
	.passthrough();

/**
 * `GetLatestArticleTimestampResponse`.
 *
 * `latest_created_at` is set only when the handler got a non-nil timestamp
 * back (handler.go: `if ts != nil`), so on the staging slice's empty
 * `articles` table the whole body is `{}`. Optional-but-typed is the honest
 * shape: a `null` here, or a body that is an array, or a `{"code":...}`
 * envelope leaking through with a 200, all fail — which is everything
 * `jsonpath "$" exists` accepted.
 *
 * The error-envelope half of that claim needs the explicit refine: an object
 * whose every field is optional accepts any object at all, so the promise in
 * the paragraph above was not true of the code until this line existed.
 */
export const latestArticleTimestampSchema = z
	.object({ latestCreatedAt: timestampSchema.optional() })
	.passthrough()
	.refine(notAnErrorEnvelope, ERROR_ENVELOPE_MESSAGE);

/** One row of `ListArticlesWithTags` / `...Forward`. */
const articleWithTagsSchema = z
	.object({
		id: z.string().optional(),
		title: z.string().optional(),
		url: z.string().optional(),
		tags: z.array(z.unknown()).optional(),
	})
	.passthrough();

/**
 * The keyset-cursor list envelope shared by ListArticlesWithTags,
 * ListArticlesWithTagsForward and (with a different cursor field)
 * ListDeletedArticles.
 *
 * `next_created_at` / `next_id` are the cursor, and they are set only when the
 * page was full — so on an empty table this too is `{}`.
 *
 * The `nextDeletedAt` refine is what actually keeps this envelope distinct
 * from `deletedArticlesPageSchema`. Without it both schemas accept both
 * bodies — they share no required field — and a handler that returned the
 * deleted-articles cursor here would satisfy either one.
 */
export const articlesWithTagsPageSchema = z
	.object({
		articles: z.array(articleWithTagsSchema).optional(),
		nextCreatedAt: timestampSchema.optional(),
		nextId: z.string().optional(),
	})
	.passthrough()
	.refine(notAnErrorEnvelope, ERROR_ENVELOPE_MESSAGE)
	.refine(
		(body) => !("nextDeletedAt" in body),
		"this page carries the deleted-articles cursor (next_deleted_at) instead of its own",
	);

export const deletedArticlesPageSchema = z
	.object({
		articles: z
			.array(z.object({ id: z.string(), deletedAt: timestampSchema }).passthrough())
			.optional(),
		nextDeletedAt: timestampSchema.optional(),
	})
	.passthrough()
	.refine(notAnErrorEnvelope, ERROR_ENVELOPE_MESSAGE)
	.refine(
		(body) => !("nextCreatedAt" in body) && !("nextId" in body),
		"this page carries the articles-with-tags cursor (next_created_at / next_id) instead of its own",
	);

/**
 * `CheckArticleExistsResponse`.
 *
 * Both fields are zero-valued for a URL nobody has stored, so a negative
 * answer is literally `{}` — and `exists: true` with no `articleId` would be
 * a contradiction the caller (pre-processor's dedupe path) would act on. The
 * refinement is what turns "200" into "200 and internally consistent".
 */
export const checkArticleExistsSchema = z
	.object({
		exists: z.boolean().optional(),
		articleId: z.string().optional(),
	})
	.passthrough()
	.refine((body) => body.exists !== true || (body.articleId ?? "") !== "", {
		message: "exists is true but articleId is missing — the caller has nothing to dedupe against",
	})
	.refine(notAnErrorEnvelope, ERROR_ENVELOPE_MESSAGE);

/** One row of `ListRecentArticles` — rag-orchestrator's reader. */
const recentArticleSchema = z
	.object({
		id: uuidSchema,
		url: z.string().min(1),
		publishedAt: timestampSchema,
		feedId: uuidSchema,
		title: z.string().optional(),
		tags: z.array(z.string()).optional(),
	})
	.passthrough();

/**
 * `ListRecentArticlesResponse` — the strongest schema in this file, and the
 * reason it is worth having one at all.
 *
 * `since` and `until` are `time.Time.Format(time.RFC3339)` (handler.go), so
 * they are non-empty strings on every answer including an empty page: protojson
 * cannot omit them. That makes them the assertion — a handler that lost its
 * window computation, or a projection that started answering `{}`, fails here
 * instead of satisfying `HTTP 200`. `count` and `articles` are omitted when
 * zero/empty, which is exactly what the staging slice's empty table produces.
 */
export const recentArticlesSchema = z
	.object({
		since: timestampSchema,
		until: timestampSchema,
		articles: z.array(recentArticleSchema).optional(),
		count: z.number().int().nonnegative().optional(),
	})
	.passthrough()
	.refine((body) => (body.count ?? 0) === (body.articles?.length ?? 0), {
		message: "count disagrees with articles.length",
	});

/**
 * `GetSystemUserResponse`.
 *
 * `user_id` is whatever Kratos handed back; the staging slice answers
 * AUTH_HUB_URL from alt-backend-deps-stub, so the value is the stub's. It is
 * still a string field the caller passes straight into a UUID column, so
 * asserting it is a non-empty string is worth more than asserting nothing.
 */
export const systemUserSchema = z
	.object({ userId: z.string().min(1) })
	.passthrough();
