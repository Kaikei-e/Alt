import { test as base } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { workerToken } from "../../_shared/ids.js";
import { env } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * All worker-scoped: an `APIRequestContext` is a connection pool, and a
 * test-scoped one would open and tear down a pool per test for no isolation
 * gain — the thing under test is a shared rag-db either way. Isolation comes
 * from **ownership** of seeded `user_id`s (see `src/seed.ts`), which is what
 * lets six workers list, read and delete concurrently without seeing each
 * other's rows.
 *
 * There is no authenticated-vs-anonymous *client* split here, unlike the
 * alt-backend suite. rag-orchestrator has no auth interceptor: identity is a
 * plain `X-Alt-User-Id` request header that every RPC's own handler reads
 * (`extractUserID`, augur/handler.go:106), and the tests deliberately vary it
 * per call — a fixture per identity would hide the fact that six different
 * users are being impersonated over one connection.
 */

type WorkerFixtures = {
	/** Unique to (dispatch, worker). Embedded in the rows this worker enqueues. */
	workerTag: string;

	/**
	 * Connect-RPC :9011, JSON codec, **no** identity header.
	 *
	 * `Connect-Protocol-Version: 1` is sent on every call because the retired
	 * Hurl files sent it on every call. connect-go v1 does not *require* it
	 * unless the handler opts into `WithRequireConnectProtocolHeader`, and
	 * whether it does is not what this suite is asserting — silently dropping
	 * the header would change the protocol under test rather than test it.
	 */
	connect: APIRequestContext;

	/** Echo REST :9010 — health, readiness, /metrics, /v1/rag/*, /internal/rag/*. */
	rest: APIRequestContext;

	/**
	 * No `baseURL`, for absolute-URL transport probes.
	 *
	 * `_shared/net.ts` asserts that a connection is *refused*; giving that probe
	 * a context whose baseURL points at a listener that does answer would make a
	 * relative-path slip silently prove the opposite of what it claims.
	 */
	probe: APIRequestContext;
};

export const test = base.extend<Record<never, never>, WorkerFixtures>({
	workerTag: [
		async ({}, use, workerInfo) => {
			await use(workerToken(workerInfo.workerIndex));
		},
		{ scope: "worker" },
	],

	connect: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.connectURL,
				extraHTTPHeaders: {
					"Content-Type": "application/json",
					"Connect-Protocol-Version": "1",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	rest: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.baseURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	probe: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext();
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],
});

export { expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Connect helpers
// ---------------------------------------------------------------------------

/** `POST /{package}.{Service}/{Method}` for a fully-qualified procedure. */
export function procedurePath(procedure: string): string {
	return procedure.startsWith("/") ? procedure : `/${procedure}`;
}

/**
 * Invokes a unary procedure **as** a specific user.
 *
 * `_shared/connect.ts`'s `callUnary` takes no per-call headers, and identity
 * here is per-call by construction: one connection impersonates six seeded
 * owners across the suite, so the header cannot live on the context. See the
 * `sharedHelperGaps` note in the migration report.
 */
export function callAs(
	api: APIRequestContext,
	procedure: string,
	userID: string,
	request: unknown = {},
): Promise<APIResponse> {
	return api.post(procedurePath(procedure), {
		headers: { "X-Alt-User-Id": userID },
		data: request,
	});
}
