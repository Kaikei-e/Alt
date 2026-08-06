import { expect } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { z } from "zod";
import { expectJson, expectStatus } from "./http.js";

/**
 * Connect-RPC (JSON codec) helpers.
 *
 * Most of Alt's inter-service surface is Connect over HTTP/1.1 with the JSON
 * codec: a unary call is `POST /{package}.{Service}/{Method}` with a JSON
 * body, and an error is a non-2xx whose body is `{"code": "...", "message":
 * "..."}` — the *string* spelling of the code, not the numeric enum the
 * connect-es client exposes (`docs/best_practices/typescript.md`, and rule 10
 * of the bp-typescript conventions: comparing `ConnectError.code` to a string
 * sends every error to the default branch). On the wire it is a string, and
 * that is what these helpers assert.
 */

/** The canonical Connect error envelope. */
export const connectErrorSchema = z
	.object({
		code: z.string().min(1),
		message: z.string().optional(),
		details: z.array(z.unknown()).optional(),
	})
	.passthrough();

export type ConnectErrorBody = z.infer<typeof connectErrorSchema>;

/**
 * The wire spellings, as Connect defines them. Kept as a const object rather
 * than free strings so a typo is a compile error instead of a test that can
 * never fail.
 */
export const ConnectCode = {
	canceled: "canceled",
	unknown: "unknown",
	invalidArgument: "invalid_argument",
	deadlineExceeded: "deadline_exceeded",
	notFound: "not_found",
	alreadyExists: "already_exists",
	permissionDenied: "permission_denied",
	resourceExhausted: "resource_exhausted",
	failedPrecondition: "failed_precondition",
	aborted: "aborted",
	outOfRange: "out_of_range",
	unimplemented: "unimplemented",
	internal: "internal",
	unavailable: "unavailable",
	dataLoss: "data_loss",
	unauthenticated: "unauthenticated",
} as const;

export type ConnectCodeName = (typeof ConnectCode)[keyof typeof ConnectCode];

/**
 * The HTTP status Connect maps each code onto, per the protocol spec. Used to
 * assert both halves of an error at once: a handler that returns the right
 * body under the wrong status breaks every generated client, and a handler
 * that returns the right status with a free-form body breaks them too.
 */
const STATUS_FOR_CODE: Readonly<Record<ConnectCodeName, number>> = {
	canceled: 499,
	unknown: 500,
	invalid_argument: 400,
	deadline_exceeded: 504,
	not_found: 404,
	already_exists: 409,
	permission_denied: 403,
	resource_exhausted: 429,
	failed_precondition: 412,
	aborted: 409,
	out_of_range: 400,
	unimplemented: 501,
	internal: 500,
	unavailable: 503,
	data_loss: 500,
	unauthenticated: 401,
};

/** Builds the unary path for a fully-qualified procedure. */
export function procedurePath(procedure: string): string {
	return procedure.startsWith("/") ? procedure : `/${procedure}`;
}

/**
 * Invokes a unary procedure with the JSON codec.
 *
 * `request` defaults to `{}` because the most common probe in these suites is
 * "does this procedure exist at all", which needs no fields — and because an
 * omitted body would make Connect answer `invalid_argument` for a reason
 * unrelated to what the test is asserting.
 */
export function callUnary(
	api: APIRequestContext,
	procedure: string,
	request: unknown = {},
): Promise<APIResponse> {
	return api.post(procedurePath(procedure), {
		headers: { "Content-Type": "application/json" },
		data: request,
	});
}

/** Invokes a procedure and asserts a 200 whose body matches `schema`. */
export async function expectUnaryOk<T>(
	api: APIRequestContext,
	procedure: string,
	request: unknown,
	schema: z.ZodType<T>,
): Promise<T> {
	const response = await callUnary(api, procedure, request);
	await expectStatus(response, 200);
	return expectJson(response, schema);
}

/**
 * Invokes a procedure and asserts it fails with exactly `code`, under the
 * HTTP status the Connect protocol pairs with that code.
 */
export async function expectUnaryError(
	api: APIRequestContext,
	procedure: string,
	request: unknown,
	code: ConnectCodeName,
): Promise<ConnectErrorBody> {
	const response = await callUnary(api, procedure, request);
	const expectedStatus = STATUS_FOR_CODE[code];

	const body = await response.text();
	expect(
		response.status(),
		`${procedure} should fail with Connect code "${code}" (HTTP ${expectedStatus})\n` +
			`body: ${body.slice(0, 1_000)}`,
	).toBe(expectedStatus);

	let parsed: unknown;
	try {
		parsed = JSON.parse(body);
	} catch {
		throw new Error(`${procedure} returned a non-JSON error body: ${body.slice(0, 1_000)}`);
	}
	const envelope = connectErrorSchema.parse(parsed);
	expect(envelope.code, `${procedure} error code`).toBe(code);
	return envelope;
}

/**
 * Asserts a procedure is **mounted**, by distinguishing 404 from everything
 * else.
 *
 * This is CLAUDE.md rule 8 ("no silent fallback for unwired dependencies")
 * pushed out to the E2E boundary. A handler whose DI dependency came back nil
 * and skipped registration, or a `mux.Handle` line lost in a refactor, is
 * invisible to unit tests and to any assertion of the form "not 2xx" — the
 * service simply stops existing and every caller gets a 404 that reads like a
 * missing row. Only the *path resolving at all* tells the two apart.
 *
 * `expected` is the status an anonymous or empty call should produce on a
 * correctly wired procedure — 401 behind an auth interceptor, or one of the
 * business outcomes when the listener is unauthenticated.
 */
export async function expectProcedureMounted(
	api: APIRequestContext,
	procedure: string,
	expected: readonly number[],
): Promise<void> {
	const response = await callUnary(api, procedure, {});
	expect(
		response.status(),
		`${procedure} answered ${response.status()}. 404 means the handler was never ` +
			`registered — check the mux wiring and the DI container, not the request. ` +
			`Expected one of [${expected.join(", ")}].\nbody: ${(await response.text()).slice(0, 500)}`,
	).not.toBe(404);
	expect(expected, `${procedure} answered ${response.status()}`).toContain(response.status());
}

/**
 * Asserts a procedure is **not** reachable on this listener.
 *
 * The mirror of `expectProcedureMounted`, for the admin/operator procedures
 * that must never appear on a user-facing port. 404 (never registered) and
 * 501/`unimplemented` are both acceptable answers; a 2xx or a 401 are not,
 * because both prove the mux resolved the path.
 */
export async function expectProcedureAbsent(
	api: APIRequestContext,
	procedure: string,
): Promise<void> {
	const response = await callUnary(api, procedure, {});
	expect(
		[404, 501],
		`${procedure} is reachable on ${response.url()} with status ${response.status()}; ` +
			`this listener must not expose it`,
	).toContain(response.status());
}
