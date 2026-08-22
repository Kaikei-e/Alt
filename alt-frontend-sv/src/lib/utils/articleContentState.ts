import { Code, ConnectError } from "@connectrpc/connect";
import { FAILURE_SCOPE_HEADER } from "./errorClassification";
import { parseRetryAfter } from "./retryAfter";

/**
 * The reading surface's honest account of one article body.
 *
 * Three of these are states the reader is told about; `idle` only exists so a
 * surface can tell "nothing asked for yet" apart from "asked, got nothing".
 *
 * The distinction that matters is `pending`/`retrying` versus `failed`. Every
 * reading surface used to collapse the first into the second — a request still
 * in flight rendered as "Source content unavailable." — so the reader was shown
 * a verdict before there was one.
 */
export type ArticleContentPhase =
	| "idle"
	| "pending"
	| "retrying"
	| "ready"
	| "failed";

/** Shown while the first request is in flight. */
export const CONTENT_PENDING_LABEL = "Fetching the full article…";

/** Shown while the single automatic re-attempt waits out its Retry-After. */
export const CONTENT_RETRYING_LABEL = "Retrying…";

/** The remedy every terminal state must offer, alongside the retry control. */
export const READ_ORIGINAL_LABEL = "Read on the original site";

/** The retry control's label, identical on every surface. */
export const TRY_AGAIN_LABEL = "Try again";

/**
 * What a reading surface's own entry point says once a body the reader never
 * asked for has come back gone.
 *
 * It keeps the noun deliberately. A background fetch that fails leaves nothing
 * on a collapsed card explaining what was attempted, so relabelling the entry
 * point to a bare `TRY_AGAIN_LABEL` asks the reader to repeat an attempt they
 * never saw — and spends the one control that says "there is an article here"
 * saying it. The verb belongs to the controls next to the notice that names
 * the problem: `TRY_AGAIN_LABEL` and `READ_ORIGINAL_LABEL`, inside the panel.
 */
export const CONTENT_UNAVAILABLE_LABEL = "Article unavailable";

/**
 * The "the server answered, with nothing" case, as an error object.
 *
 * `content: ""` is a state and never a falsy no-op — treating it as "not yet
 * fetched" is what spawned ADR-000581's infinite `$effect`. Giving it an error
 * value lets every surface reach the terminal state through the one formatter
 * (`articleContentErrorMessage`) instead of re-hardcoding a literal, and
 * `Code.DataLoss` keeps it out of the transient set, which is correct: an empty
 * body is not going to fill itself in half a second.
 */
export const EMPTY_CONTENT_ERROR = new ConnectError(
	"article content was empty",
	Code.DataLoss,
);

/** Floor for the foreground wait: below this the "Retrying…" state cannot be read. */
export const FOREGROUND_RETRY_MIN_WAIT_MS = 500;

/**
 * Ceiling for the foreground wait. `RATE_LIMIT_EXTERNAL_API_INTERVAL` defaults
 * to 7.5s per host ([[000982]] cut it from the 10s [[000959]] recorded), and
 * the server rounds that up to an 8s `Retry-After` (RFC 9110 delta-seconds are
 * integers), so a legitimate slot wait still fits underneath this ceiling;
 * anything larger is not something to hold a reading surface open for.
 */
export const FOREGROUND_RETRY_MAX_WAIT_MS = 10_000;

/** Used when the server named a wait we cannot parse, or named none at all. */
export const FOREGROUND_RETRY_DEFAULT_WAIT_MS = 2_000;

/**
 * How long to wait before the ONE automatic foreground re-attempt, or null when
 * there must not be one.
 *
 * Deliberately narrower than `isRetryableContentError`. That predicate gates a
 * jittered, budget-capped BACKGROUND retry inside the prefetch ladder and
 * admits `Code.Unavailable`; this one runs on the surface the reader is looking
 * at, and ADR-000959 explicitly rejected putting `Code.Unavailable` on a
 * foreground auto-retry path — two components re-sending into an open circuit
 * breaker was the incident that ADR was written to close. It stays rejected.
 *
 * What is admitted is the single case ADR-000959 §4 carved out and ADR-000963
 * §2 made legible: a `ResourceExhausted` with no `X-Alt-Failure-Scope: host`
 * stamp. Unstamped means alt-backend attributed the failure to its own
 * host-slot politeness gate rather than to the publisher, so not one packet
 * left the building, nothing downstream is being hammered, and the server
 * already said in `Retry-After` when the slot frees. A stamped one is the
 * publisher itself answering 429 and must not be re-sent into.
 */
export function foregroundRetryDelayMs(err: unknown): number | null {
	const connectErr = ConnectError.from(err);
	if (connectErr.code !== Code.ResourceExhausted) return null;
	if (connectErr.metadata.get(FAILURE_SCOPE_HEADER) === "host") return null;

	const wait =
		parseRetryAfter(connectErr.metadata.get("Retry-After")) ??
		FOREGROUND_RETRY_DEFAULT_WAIT_MS;
	return Math.min(
		FOREGROUND_RETRY_MAX_WAIT_MS,
		Math.max(FOREGROUND_RETRY_MIN_WAIT_MS, wait),
	);
}
