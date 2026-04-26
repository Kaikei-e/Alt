/**
 * Connect-RPC error helpers for BFF server routes.
 *
 * @connectrpc/connect v2 `ConnectError.code` is the numeric `Code` enum
 * (1..16), not a string — the public type is
 * `readonly code: Code` (see `@connectrpc/connect/dist/esm/code.d.ts`).
 * Network failures (fetch TypeError, DNS, ECONNREFUSED before the response is
 * parsed) surface as a plain `Error` with no `code` — those must be treated
 * as upstream-unreachable by callers.
 *
 * For back-compat with hand-crafted test mocks that set `code` as a string,
 * string values are accepted and lowercased. Unknown shapes return undefined.
 *
 * Full code list: https://connectrpc.com/docs/protocol#error-codes
 */
import { Code } from "@connectrpc/connect";

export type ConnectCode =
	| "canceled"
	| "unknown"
	| "invalid_argument"
	| "deadline_exceeded"
	| "not_found"
	| "already_exists"
	| "permission_denied"
	| "resource_exhausted"
	| "failed_precondition"
	| "aborted"
	| "out_of_range"
	| "unimplemented"
	| "internal"
	| "unavailable"
	| "data_loss"
	| "unauthenticated";

// Exhaustive map keyed on the numeric Code enum. Kept inline instead of
// reaching into @connectrpc/connect's private `codeToString` helper so the
// mapping is stable under minor-version upgrades.
const CODE_TO_STRING: Readonly<Record<number, ConnectCode>> = {
	[Code.Canceled]: "canceled",
	[Code.Unknown]: "unknown",
	[Code.InvalidArgument]: "invalid_argument",
	[Code.DeadlineExceeded]: "deadline_exceeded",
	[Code.NotFound]: "not_found",
	[Code.AlreadyExists]: "already_exists",
	[Code.PermissionDenied]: "permission_denied",
	[Code.ResourceExhausted]: "resource_exhausted",
	[Code.FailedPrecondition]: "failed_precondition",
	[Code.Aborted]: "aborted",
	[Code.OutOfRange]: "out_of_range",
	[Code.Unimplemented]: "unimplemented",
	[Code.Internal]: "internal",
	[Code.Unavailable]: "unavailable",
	[Code.DataLoss]: "data_loss",
	[Code.Unauthenticated]: "unauthenticated",
};

/**
 * Extract a Connect-RPC error code from a caught value.
 * - ConnectError (numeric `code`): translated to snake_case string.
 * - Objects with a string `code` (pre-lowercased in tests): returned lowercased.
 * - Everything else (bare Error, null, primitives): returns undefined.
 */
export function extractConnectCode(err: unknown): ConnectCode | undefined {
	if (!err || typeof err !== "object" || !("code" in err)) return undefined;
	const c = (err as { code: unknown }).code;
	if (typeof c === "number") {
		return CODE_TO_STRING[c];
	}
	if (typeof c === "string") {
		const lower = c.toLowerCase();
		return (
			(CODE_TO_STRING as unknown as Record<string, ConnectCode>)[lower] ??
			(lower in reverseIndex ? (lower as ConnectCode) : undefined)
		);
	}
	return undefined;
}

// Precompute allowed string codes for quick membership check.
const reverseIndex: Readonly<Record<string, true>> = Object.freeze(
	Object.fromEntries(
		Object.values(CODE_TO_STRING).map((s) => [s, true as const]),
	),
);
