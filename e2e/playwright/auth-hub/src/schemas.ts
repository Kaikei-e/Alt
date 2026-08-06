import { z } from "zod";
import { timestampSchema, uuidSchema } from "../../_shared/schemas.js";

/**
 * Response envelopes, as Zod schemas.
 *
 * The Hurl suite asserted one JSONPath at a time — `jsonpath "$.session.id"
 * exists` passes for `null`, `""`, `[]` and `{"error": …}` alike — so a
 * handler that changed shape satisfied it silently. A schema asserts the whole
 * envelope in one step.
 *
 * `passthrough()` is the default: these are contracts on the fields auth-hub
 * promises, not a freeze on the ones it may add. `strict()` appears exactly
 * twice below, and in both places an *extra* field is itself the bug.
 */

export {
	uuidSchema,
	timestampSchema,
	/** Handlers that answer via `echo.NewHTTPError` — see `echoErrorSchema` below. */
	plainErrorSchema,
} from "../../_shared/schemas.js";

/**
 * `GET /health` — and `strict()` is the whole point.
 *
 * `01-health.hurl` carried the reason in a comment: auth-hub has two health
 * handlers, and `internal/adapter/handler/health.go` (the one `main.go`
 * mounts) emits `{"status":"healthy"}` and nothing else, while the legacy
 * `handler/health_handler.go` adds a `service` field. A `passthrough()` schema
 * would accept either, so "main.go wired the wrong handler" — a real
 * regression, and precisely the kind a refactor introduces — would go
 * unnoticed. The extra key is the signal.
 */
export const healthSchema = z.object({ status: z.literal("healthy") }).strict();

/**
 * `GET /internal/system-user` — also `strict()`.
 *
 * `internal.go`'s `systemUserResponse` has exactly one field. This is a
 * service-to-service endpoint guarded only by a shared secret, so any
 * additional key here is identity data leaking across a trust boundary that
 * has no per-user authorization behind it. An extra field is the bug.
 */
export const systemUserSchema = z.object({ user_id: uuidSchema }).strict();

/**
 * `GET /session` — the shape the SPA's session bootstrap reads.
 *
 * Field-by-field against `internal/adapter/handler/session.go:23-43`:
 *
 * - `user.role` is `z.enum` rather than `z.string()`: the Kratos gateway
 *   normalises anything unrecognised down to `"user"` and only promotes an
 *   exact `"admin"` trait (`gateway/kratos.go`). A third value on the wire
 *   would mean that normalisation was bypassed, which is a privilege question.
 * - `user.lastLoginAt` is required despite carrying `,omitempty` in the Go
 *   tag: `omitempty` does not elide a non-zero `time.Time` struct, and the
 *   handler assigns `time.Now()` unconditionally. Making it required pins that
 *   the SPA can rely on the field existing.
 * - `session.id` is only `min(1)`, not `uuidSchema`. See
 *   tests/session.spec.ts — the handler currently returns the raw
 *   `ory_kratos_session` cookie value here, not the Kratos session UUID, and
 *   asserting either shape would encode a claim this suite should not be the
 *   one to settle.
 */
export const sessionResponseSchema = z
	.object({
		ok: z.literal(true),
		user: z
			.object({
				id: uuidSchema,
				tenantId: uuidSchema,
				email: z.string().min(3),
				role: z.enum(["user", "admin"]),
				createdAt: timestampSchema,
				lastLoginAt: timestampSchema,
			})
			.passthrough(),
		session: z
			.object({
				id: z.string().min(1),
				active: z.literal(true),
			})
			.passthrough(),
	})
	.passthrough();

/**
 * `POST /csrf` — `{"data":{"csrf_token":"<unix>.<b64url(mac)>"}}`.
 *
 * `07-csrf-happy.hurl` asserted `matches "^[A-Za-z0-9_+/=.-]{16,}$"`, which
 * accepts any 16-character blob including one with no separator at all. The
 * real format is fixed by `infrastructure/token/csrf.go`:
 * `fmt.Sprintf("%d.%s", ts, base64.URLEncoding.EncodeToString(mac))` where
 * `mac` is HMAC-SHA256 — 32 bytes, which `base64.URLEncoding` (padded)
 * renders as 43 characters plus one `=`.
 *
 * Pinning the length is what catches a truncated MAC: a generator that
 * switched to the first 8 bytes of the digest still matches the old regex and
 * still round-trips through `Validate`, while having lost 192 bits of
 * forgery resistance.
 */
export const csrfTokenPattern = /^\d{10}\.[A-Za-z0-9_-]{43}=$/;

export const csrfResponseSchema = z
	.object({
		data: z.object({ csrf_token: z.string().regex(csrfTokenPattern) }).passthrough(),
	})
	.passthrough();

/**
 * Echo's `DefaultHTTPErrorHandler` envelope: `{"message": "..."}`.
 *
 * Every auth-hub error path except one goes through `echo.NewHTTPError`
 * (`error_mapper.go`, `validate.go`, `session.go`, `middleware/
 * internal_auth.go`, `middleware/rate_limit.go`), and Echo renders those as a
 * `message` key — not `error`. The SPA's fetch wrapper branches on the
 * envelope, so which key appears is a contract even though the phrasing is
 * not: this schema deliberately asserts only that the key exists and is
 * non-empty, matching the Hurl suite's stated position that "body phrasing
 * belongs to Kratos and to mapDomainError".
 */
export const echoErrorSchema = z.object({ message: z.string().min(1) }).passthrough();

/**
 * `POST /csrf` without a Cookie header is the one exception.
 *
 * `csrf.go:34-40` short-circuits with `c.JSON(401, map[string]string{"error":
 * …})` rather than `echo.NewHTTPError`, so this single path answers with an
 * `error` key while the identical-looking 401 from a *bad* cookie answers with
 * `message`. Two schemas rather than a union because a spec asserting the
 * wrong one is the finding.
 */
export const csrfMissingCookieErrorSchema = z.object({ error: z.string().min(1) }).passthrough();

/* ------------------------------------------------------------------------ *
 * Kratos — not the system under test, but the suite's seeding surface.
 * A malformed answer here has to fail as "seeding broke", never as "auth-hub
 * is wrong", which is what these schemas buy.
 * ------------------------------------------------------------------------ */

export const kratosIdentitySchema = z
	.object({
		id: uuidSchema,
		schema_id: z.string().min(1),
		state: z.string().min(1),
		traits: z.object({ email: z.string().min(3) }).passthrough(),
	})
	.passthrough();

const kratosUINodeSchema = z
	.object({
		attributes: z
			.object({
				name: z.string().optional(),
				value: z.unknown().optional(),
			})
			.passthrough(),
	})
	.passthrough();

export const kratosLoginFlowSchema = z
	.object({
		id: z.string().min(1),
		ui: z
			.object({
				action: z.string().url(),
				method: z.string().min(1),
				nodes: z.array(kratosUINodeSchema).min(1),
			})
			.passthrough(),
	})
	.passthrough();

export const kratosLoginResultSchema = z
	.object({
		session: z
			.object({
				id: uuidSchema,
				active: z.literal(true),
				identity: z
					.object({
						id: uuidSchema,
						traits: z.object({ email: z.string().min(3) }).passthrough(),
					})
					.passthrough(),
			})
			.passthrough(),
	})
	.passthrough();
