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

/**
 * Primitives every suite shares live in `_shared/schemas.ts`; they are
 * re-exported here so a spec keeps one import for "the shapes alt-backend
 * answers with" and does not have to know which of them are fleet-wide.
 */
export {
	uuidSchema,
	timestampSchema,
	/** `utils/errors.SecureHTTPResponse` — what HandleError writes for a typed AppContextError. */
	secureErrorSchema,
	/** Handlers answering with `echo.NewHTTPError` write a bare `{"error": "..."}`. */
	plainErrorSchema,
} from "../../_shared/schemas.js";
export { connectErrorSchema } from "../../_shared/connect.js";
import { uuidSchema } from "../../_shared/schemas.js";

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
 * The cursor envelope — in its two incompatible forms.
 *
 * There is no single shared envelope, which is what the first CI run of this
 * suite established. `/v1/feeds/fetch/cursor` builds an
 * `ArticlesWithCursorResponse` (`{data, has_more, next_cursor?}`), while
 * `/fetch/viewed/cursor` and `/fetch/favorites/cursor` assemble a bare
 * `map[string]interface{}` with `data` and — only when the page is non-empty —
 * `next_cursor`. They have **no `has_more` field at all**
 * (rest_feeds/fetch.go: RestHandleFetchReadFeedsCursor /
 * RestHandleFetchFavoriteFeedsCursor).
 *
 * The Hurl suite could not see this: it asserted `jsonpath "$" exists` on the
 * viewed and favorites endpoints, which is true of `{"data":[]}` and of
 * everything else.
 *
 * `next_cursor` is `*string` with `omitempty` on the typed side and a
 * conditionally-set map key on the untyped side, so in both forms it is
 * *absent*, never null, on a final page.
 */
export const fullCursorPageSchema = z
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

/**
 * The viewed / favorites form. `has_more` is asserted absent rather than
 * merely optional: making it optional would let the two shapes silently
 * converge or diverge without the suite noticing, which is the whole thing
 * this pair of schemas exists to prevent.
 */
export const dataOnlyCursorPageSchema = z
	.object({
		data: z.array(z.unknown()).nullable(),
		next_cursor: z.string().nullable().optional(),
		has_more: z.undefined(),
	})
	.passthrough();

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
