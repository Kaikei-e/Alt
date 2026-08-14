/**
 * Positive-form validation for the Connect-RPC paths our proxies forward.
 *
 * Both `/api/v2` proxies attach a privileged token to a path the caller chose,
 * so the segment they append to an upstream origin decides which endpoint that
 * token reaches. SvelteKit hands rest params to the route already
 * percent-decoded, so `%5C` arrives as a literal `\` and `%2E%2E` as `..`; the
 * WHATWG URL parser inside `fetch` then treats `\` as a path separator for
 * http(s) and collapses dot segments. A negative guard — splitting on "/",
 * scanning for "..", blacklisting characters — is therefore always one encoding
 * behind: the value it inspects is not the value `fetch` will resolve.
 *
 * So we validate in the positive direction instead. A Connect path is exactly
 * `<package>.<Service>/<Method>`, and both halves are proto identifiers, which
 * cannot contain a separator, a dot segment, a percent sign or a query/fragment
 * delimiter in the first place. Anything that fails the identifier shape is
 * refused without asking why it failed.
 */

/**
 * A single proto identifier: a leading letter, then letters, digits and
 * underscores. Deliberately anchored — an unanchored test would match the
 * identifier-looking prefix of `Watch\..\..\v1\aggregate`.
 */
const IDENTIFIER = /^[A-Za-z][A-Za-z0-9_]*$/;

/**
 * A fully-qualified Connect service name: dot-joined proto identifiers, e.g.
 * `alt.knowledge_home.v1.KnowledgeHomeService`.
 */
const SERVICE_NAME = /^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)*$/;

export interface ConnectPath {
	readonly service: string;
	readonly method: string;
}

/**
 * True when `value` is a well-formed Connect method name (one proto identifier).
 *
 * Use this where the service prefix is fixed by the route and only the method
 * comes from the caller.
 */
export function isConnectMethodName(value: string): boolean {
	return IDENTIFIER.test(value);
}

/**
 * Parse `<service>/<method>` into its two halves, or null when the input is not
 * exactly that shape.
 *
 * Returning null rather than throwing keeps the caller's refusal path a plain
 * 404: the proxy must not tell an unauthenticated prober which half it rejected.
 */
export function parseConnectPath(path: string): ConnectPath | null {
	const separator = path.indexOf("/");
	if (separator === -1) return null;

	const service = path.slice(0, separator);
	const method = path.slice(separator + 1);

	if (!SERVICE_NAME.test(service)) return null;
	if (!isConnectMethodName(method)) return null;

	return { service, method };
}
