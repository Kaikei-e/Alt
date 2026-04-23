/**
 * Connect-RPC error helpers for BFF server routes.
 *
 * connect-web throws `ConnectError` instances with a lowercased `code` property
 * (e.g. "unavailable", "internal", "deadline_exceeded") whenever the wire response
 * carries a Connect-RPC error envelope. Network failures (fetch TypeError, DNS
 * resolution failure, ECONNREFUSED before a response is parsed) surface as plain
 * `Error` with no `code` — the caller must treat these as upstream unreachable.
 *
 * Full code list: https://connectrpc.com/docs/protocol#error-codes
 */

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

/**
 * Extract a Connect-RPC error code from a caught value. Returns undefined when
 * the error is a bare JS Error (network-level failure before a Connect
 * response was parsed) or when the `code` property is not a string.
 */
export function extractConnectCode(err: unknown): string | undefined {
	if (err && typeof err === "object" && "code" in err) {
		const c = (err as { code: unknown }).code;
		return typeof c === "string" ? c.toLowerCase() : undefined;
	}
	return undefined;
}
