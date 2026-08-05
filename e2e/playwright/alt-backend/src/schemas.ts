import { z } from "zod";

/**
 * Response envelopes, as Zod schemas.
 *
 * Hurl could only assert one JSONPath at a time, which is why the suite it
 * replaced is full of `jsonpath "$" exists` — an assertion that passes for
 * `null`, `[]`, `{}` and `{"error": ...}` alike. A schema asserts the *whole*
 * envelope in one step, so a handler that starts returning a different shape
 * fails here instead of silently satisfying a spot check.
 *
 * Every object schema is `passthrough()`: this is a contract on the fields
 * alt-backend promises, not a freeze on the ones it may add.
 */

/** RFC 4122 textual UUID, any version. */
export const uuidSchema = z
	.string()
	.regex(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);

export const healthSchema = z
	.object({
		status: z.literal("healthy"),
		database: z.literal("connected"),
	})
	.passthrough();

export const csrfTokenSchema = z
	.object({
		// base64url alphabet — the driver encodes 32 random bytes.
		csrf_token: z.string().regex(/^[A-Za-z0-9_\-=+/]+$/),
	})
	.passthrough();

/**
 * `utils/errors.SecureHTTPResponse` — what HandleError writes for every typed
 * AppContextError.
 */
export const secureErrorSchema = z
	.object({
		error: z
			.object({
				code: z.string().min(1),
				message: z.string(),
				error_id: z.string().optional(),
				retryable: z.boolean().optional(),
			})
			.passthrough(),
	})
	.passthrough();

/** Handlers that answer with echo.NewHTTPError write a bare `{"error": "..."}`. */
export const plainErrorSchema = z
	.object({ error: z.string() })
	.passthrough();

export const rssFeedLinkSchema = z
	.object({
		id: uuidSchema,
		url: z.string().url(),
		health_status: z.unknown(),
	})
	.passthrough();

export const rssFeedLinkListSchema = z.array(rssFeedLinkSchema);

export const feedItemSchema = z
	.object({
		link: z.string(),
	})
	.passthrough();

export const feedListSchema = z.array(feedItemSchema);

/**
 * The shared cursor envelope: `{data, has_more, next_cursor?}`.
 *
 * `next_cursor` is `*string` with `omitempty` in Go, so it is absent — not
 * null — when the page is the last one. The Hurl suite asserted it always
 * "exists", which only held because the seeded fixture happened to produce a
 * non-final page. The invariant that actually holds for every page is the
 * refinement below: a page that claims `has_more` must hand back a cursor to
 * get the next one.
 */
export const cursorPageSchema = z
	.object({
		data: z.array(z.unknown()).nullable(),
		has_more: z.boolean(),
		next_cursor: z.string().nullable().optional(),
	})
	.passthrough()
	.refine(
		(page) => !page.has_more || (typeof page.next_cursor === "string" && page.next_cursor !== ""),
		{ message: "has_more is true but next_cursor is missing or empty" },
	);

export const unreadCountSchema = z
	.object({ count: z.number().int().nonnegative() })
	.passthrough();

export const feedTagsByURLSchema = z
	.object({
		feed_url: z.string(),
		tags: z.array(z.unknown()),
	})
	.passthrough();

export const feedTagsByIDSchema = z
	.object({
		feed_id: z.string(),
		tags: z.array(z.unknown()),
	})
	.passthrough();

export const articleTagsSchema = z
	.object({
		article_id: z.string(),
		tags: z.array(z.unknown()),
	})
	.passthrough();

export const articlesByTagSchema = z
	.object({
		articles: z.array(z.unknown()),
		has_more: z.boolean(),
	})
	.passthrough();

const amountSchema = z.object({ amount: z.number().int().nonnegative() }).passthrough();

export const feedStatsSchema = z
	.object({
		feed_amount: amountSchema,
		summarized_feed: amountSchema,
	})
	.passthrough();

export const detailedFeedStatsSchema = z
	.object({
		feed_amount: amountSchema,
		total_articles: amountSchema,
		unsummarized_articles: amountSchema,
	})
	.passthrough();

/** Connect-RPC error envelope (connectrpc.com/docs/protocol#error-end-stream). */
export const connectErrorSchema = z
	.object({
		code: z.string().min(1),
		message: z.string().optional(),
	})
	.passthrough();

/**
 * `KnowledgeHomeAdminService.EmitArticleUrlBackfill`.
 *
 * The operator listener installs `codec.EmitUnpopulatedJSONCodec`, precisely
 * so zero counters stay on the wire instead of being stripped by protojson —
 * which is what makes `z.number()` (not `.optional()`) the right assertion and
 * what this schema is here to keep honest.
 */
export const articleUrlBackfillSchema = z
	.object({
		articlesScanned: z.number().int().nonnegative(),
		eventsAppended: z.number().int().nonnegative(),
		skippedBlockedScheme: z.number().int().nonnegative(),
		skippedDuplicate: z.number().int().nonnegative(),
		moreRemaining: z.boolean(),
	})
	.passthrough();

export const registerFeedSchema = z
	.object({ message: z.string() })
	.passthrough();

export const augurAnswerSchema = z
	.object({ answer: z.string() })
	.passthrough();
