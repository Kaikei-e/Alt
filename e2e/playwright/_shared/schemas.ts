import { z } from "zod";

/**
 * Schema primitives shared across suites.
 *
 * Hurl could only assert one JSONPath at a time, which is why the suites these
 * replace are full of `jsonpath "$" exists` — an assertion that passes for
 * `null`, `[]`, `{}` and `{"error": ...}` alike. A schema asserts the *whole*
 * envelope in one step, so a handler that starts returning a different shape
 * fails here instead of silently satisfying a spot check.
 *
 * Every object schema is `passthrough()` by convention: these are contracts on
 * the fields a service promises, not a freeze on the ones it may add. Use
 * `.strict()` only where an extra field is itself the bug — an endpoint that
 * must not leak internal identifiers, for instance.
 */

/** RFC 4122 textual UUID, any version. */
export const uuidSchema = z
	.string()
	.regex(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);

/**
 * RFC 3339 / ISO 8601 instant.
 *
 * Deliberately not `z.string().datetime()`: that rejects the `+09:00` offset
 * form, which protojson and several of Alt's Go handlers emit.
 */
export const timestampSchema = z
	.string()
	.regex(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/);

/** `YYYY-MM-DD`, for the date-partitioned digest and recap surfaces. */
export const dateSchema = z.string().regex(/^\d{4}-\d{2}-\d{2}$/);

/**
 * The two health-envelope shapes across the fleet.
 *
 * Go services answer `{"status":"healthy",...}`; several Python and Rust
 * services answer `{"status":"ok"}`. A union rather than a per-suite copy
 * keeps "this is a health response" one idea, while still refusing the
 * `{"status":"degraded"}` that a service under a broken dependency emits.
 */
export const healthStatusSchema = z.enum(["healthy", "ok", "UP", "SERVING"]);

export const healthSchema = z.object({ status: healthStatusSchema }).passthrough();

/**
 * A non-empty error envelope in the shape Alt's Go handlers write for a typed
 * AppContextError (`utils/errors.SecureHTTPResponse`).
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

/** Handlers that answer with `echo.NewHTTPError` write a bare `{"error": "..."}`. */
export const plainErrorSchema = z.object({ error: z.string() }).passthrough();

/** FastAPI's default validation envelope (news-creator, tag-generator, metrics). */
export const fastapiErrorSchema = z
	.object({
		detail: z.union([
			z.string(),
			z.array(
				z
					.object({
						loc: z.array(z.union([z.string(), z.number()])),
						msg: z.string(),
						type: z.string(),
					})
					.passthrough(),
			),
		]),
	})
	.passthrough();

/**
 * Asserts a list is a list *and* that its elements are what we think.
 *
 * `z.array(x)` alone accepts `[]`, which is the single most common way an
 * assertion about a collection stops testing anything: a projector that
 * stopped writing rows returns `[]` forever and every `is an array` check
 * still passes. Use this where the scenario seeded the data it is reading
 * back.
 */
export function nonEmptyArray<T>(element: z.ZodType<T>): z.ZodType<T[]> {
	return z.array(element).min(1, "expected at least one element, got an empty list");
}
