import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { testToken, uuid } from "../../_shared/ids.js";
import { env } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * recap-worker has no authentication and no tenant dimension — the staging
 * slice runs `MTLS_ENFORCE=false`, `PEER_IDENTITY_TRUSTED=off`, and no handler
 * in `api::router` reads a caller identity — so there is exactly one client
 * here, worker-scoped. `APIRequestContext` is a connection pool; rebuilding it
 * per test would add a TCP handshake to every assertion for no isolation gain.
 *
 * The client sets **no default Content-Type**. Several specs deliberately post
 * the wrong one to prove a route is mounted through axum's `Json` extractor
 * rejection (415 before the handler runs), and a context-level default that
 * every request inherited would make those probes silently correct themselves.
 *
 * Isolation: what can be isolated by naming is. `seed` mints a per-test token
 * that goes into search terms, dataset paths and letter ids, so every test
 * that asks "is my thing there" asks about a thing only it created. What
 * genuinely cannot be isolated — the latest completed recap for a window, the
 * pipeline's `Semaphore(1)` run lock — lives in its own project rather than
 * pretending; see playwright.config.ts.
 */

type WorkerFixtures = {
	/** The one plaintext axum listener, :9005. */
	api: APIRequestContext;
};

export type Seed = {
	/** Unique to (dispatch, worker, test). Safe in a query string or a path. */
	readonly token: string;
	/**
	 * A fresh v4 UUID that no row in recap_db carries.
	 *
	 * Used for the "well-formed identifier, no such record" half of every
	 * path-parameter contract — the answer that must be 404 or an empty list,
	 * never the 400 a malformed identifier gets.
	 */
	readonly absentId: string;
	/**
	 * A search term guaranteed to match nothing, ever.
	 *
	 * `search_recaps_by_term` matches against `top_terms`, which the stub fills
	 * with `["ai", "model", "inference"]`. A token derived from the run id can
	 * never collide with that, or with a sibling worker's token — which is what
	 * makes "exactly zero results" an assertion rather than a race.
	 */
	readonly absentTerm: string;
};

type TestFixtures = {
	seed: Seed;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	api: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.baseURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	seed: async ({}, use, testInfo) => {
		const token = testToken(testInfo.workerIndex, testInfo.title);
		await use({ token, absentId: uuid(), absentTerm: `zz-absent-${token}` });
	},
});

export { expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

/**
 * `GenerateRecapRequest`. `genres` is `Option<Vec<String>>` with
 * `#[serde(default)]`, so omitting it is the "use the configured defaults"
 * path and `[]`/`["", "  "]` are the rejection path.
 */
export function triggerBody(genres?: readonly string[]): Record<string, unknown> {
	return genres === undefined ? {} : { genres: [...genres] };
}

/**
 * `GenreLearningRequest` as recap-subworker posts it (api/learning.rs:8-32).
 *
 * `summary` is a required field — omitting it is a serde data error, not the
 * "no configuration values" 400 — so both the accepted and the rejected body
 * carry it and differ only in whether any threshold is present.
 */
export function genreLearningBody(
	overrides: Record<string, number> | null,
): Record<string, unknown> {
	const body: Record<string, unknown> = { summary: {} };
	if (overrides !== null) body["graph_override"] = overrides;
	return body;
}
