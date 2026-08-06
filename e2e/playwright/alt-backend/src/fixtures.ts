import { randomBytes, randomInt } from "node:crypto";
import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { env } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * Everything expensive here is **worker-scoped**: the HTTP clients, the CSRF
 * token (server-side, 1h TTL, not consumed on use) and the per-worker feed
 * seed. Test-scoped copies would re-register three RSS feeds per test — each
 * one an outbound fetch through the SSRF guard — for no isolation gain,
 * because the thing under test is a shared Postgres either way.
 *
 * Isolation comes from *naming*, not from teardown: each worker seeds feeds
 * under a URL prefix nothing else uses, so two workers can register, list and
 * delete concurrently without ever seeing each other's rows. That is what
 * makes `fullyParallel` safe here, and it is the one property the Hurl suite
 * did not have — `13-rss-feed-link-delete.hurl` deleted `$[0].id`, whichever
 * feed that happened to be.
 */

export type SeededFeed = {
	/** The URL registered with alt-backend (also the stub path it serves). */
	readonly url: string;
	/** Slug embedded in the URL; unique to this worker. */
	readonly slug: string;
};

type WorkerFixtures = {
	/**
	 * Synthetic client address sent as `X-Real-IP` on every REST request.
	 *
	 * alt-backend's DoS middleware runs with `trustForwardedHeaders=true`
	 * (routes.go: it is always deployed behind an nginx that rewrites the
	 * header), keys its token bucket on the client IP, and blocks an IP for
	 * five minutes once the bucket empties — 100 req/min sustained, burst 200.
	 * Six workers sharing the runner container's single source address would
	 * drain one shared bucket and the whole suite would 429 into a 5-minute
	 * hole partway through.
	 *
	 * Giving each worker its own address is not a workaround for the limiter;
	 * it is what production actually looks like, where every browser is a
	 * distinct X-Real-IP. The limiter itself is asserted on purpose, against a
	 * dedicated address, in tests/rate-limit.spec.ts.
	 *
	 * 100.64.0.0/10 is RFC 6598 shared address space: routable-looking enough
	 * for `net.ParseIP`, guaranteed never to be a real client.
	 */
	clientIP: string;

	/** REST :9000 with JWT + per-worker client IP. */
	rest: APIRequestContext;
	/** REST :9000 with the client IP but no JWT — for auth negatives. */
	restAnon: APIRequestContext;
	/** User-facing Connect-RPC :9101 with JWT. */
	connect: APIRequestContext;
	/** User-facing Connect-RPC :9101 without JWT — for auth negatives. */
	connectAnon: APIRequestContext;
	/** Loopback operator listener :9102 (admin Connect services). */
	operator: APIRequestContext;
	/** Shared ops listener :9110 (/health + /metrics). */
	ops: APIRequestContext;

	/** A CSRF token valid for this worker's state-changing requests. */
	csrf: string;

	/** Three RSS feeds registered under this worker's private URL prefix. */
	seededFeeds: readonly SeededFeed[];
};

/** 12 hex characters: enough to keep workers, shards and reruns apart. */
function workerToken(workerIndex: number): string {
	// Crypto randomness rather than Math.random(): this token is what keeps
	// parallel workers from colliding in a shared database, and Playwright
	// workers are separate processes with no shared PRNG state.
	const random = randomBytes(3).toString("hex");
	return `${env.runId}-w${workerIndex}-${random}`.toLowerCase().replace(/[^a-z0-9-]/g, "");
}

/** Feed/article URL on the one host alt-backend's SSRF allowlist admits. */
export function stubURL(path: string): string {
	return `http://${env.stubHost}/alt-backend/e2e/${path}`;
}

export const test = base.extend<Record<never, never>, WorkerFixtures>({
	clientIP: [
		async ({}, use, workerInfo) => {
			// Deterministic in the second octet (worker index) so a failure log
			// points back at a worker, random in the last two so a rerun against
			// a still-warm backend never inherits the previous run's bucket.
			const second = 64 + (workerInfo.workerIndex % 64);
			const third = randomInt(0, 256);
			const fourth = 1 + randomInt(0, 254);
			await use(`100.${second}.${third}.${fourth}`);
		},
		{ scope: "worker" },
	],

	rest: [
		async ({ playwright, clientIP }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: {
					"X-Alt-Backend-Token": env.jwt,
					"X-Real-IP": clientIP,
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	restAnon: [
		async ({ playwright, clientIP }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: { "X-Real-IP": clientIP },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	connect: [
		async ({ playwright, clientIP }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.connectURL,
				extraHTTPHeaders: {
					"X-Alt-Backend-Token": env.jwt,
					"X-Real-IP": clientIP,
					"Content-Type": "application/json",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	connectAnon: [
		async ({ playwright, clientIP }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.connectURL,
				extraHTTPHeaders: {
					"X-Real-IP": clientIP,
					"Content-Type": "application/json",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	operator: [
		async ({ playwright, clientIP }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.internalURL,
				extraHTTPHeaders: {
					"X-Alt-Backend-Token": env.jwt,
					"X-Real-IP": clientIP,
					"Content-Type": "application/json",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	ops: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.opsURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	csrf: [
		async ({ rest }, use) => {
			const response = await rest.get("/v1/csrf-token");
			if (!response.ok()) {
				throw new Error(
					`GET /v1/csrf-token returned ${response.status()}; every ` +
						`state-changing scenario in this suite depends on it.`,
				);
			}
			const body: unknown = await response.json();
			const token =
				typeof body === "object" && body !== null
					? (body as Record<string, unknown>)["csrf_token"]
					: undefined;
			if (typeof token !== "string" || token === "") {
				throw new Error(`GET /v1/csrf-token returned no token: ${JSON.stringify(body)}`);
			}
			// The token is minted server-side with a 1h TTL and is not consumed on
			// use (csrf_token_gateway.go), so one per worker is safe for any run
			// short of an hour — which a 30s-timeout API suite always is.
			await use(token);
		},
		{ scope: "worker" },
	],

	seededFeeds: [
		async ({ rest, csrf }, use, workerInfo) => {
			const token = workerToken(workerInfo.workerIndex);
			const feeds: SeededFeed[] = [1, 2, 3].map((n) => {
				const slug = `feed-${token}-${n}`;
				return { slug, url: stubURL(`${slug}.xml`) };
			});

			for (const feed of feeds) {
				const response = await rest.post("/v1/rss-feed-link/register", {
					headers: { "X-CSRF-Token": csrf, "Content-Type": "application/json" },
					data: { url: feed.url },
				});
				if (!response.ok()) {
					throw new Error(
						`seeding ${feed.url} failed with ${response.status()}: ` +
							`${await response.text()}`,
					);
				}
			}

			// No teardown on purpose. The staging slice is created and destroyed
			// per dispatch (`docker compose down -v` in run.sh), so deleting rows
			// here would only buy a race with tests/rss-feed-link.spec.ts, which
			// registers and deletes feeds of its own under the same prefix.
			await use(feeds);
		},
		{ scope: "worker" },
	],
});

export { expect } from "@playwright/test";
