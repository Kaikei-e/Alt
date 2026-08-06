/**
 * Identifier minting for parallel-safe seeding.
 *
 * Isolation in these suites comes from **naming**, not from teardown. Each
 * worker seeds rows under a token nothing else uses, so N workers can create,
 * list and delete concurrently against one shared Postgres without ever
 * seeing each other's data — which is what makes `fullyParallel` safe.
 *
 * Teardown would not buy the same property: a suite that deletes what it made
 * still races with a sibling worker's `list`, and the staging stack is torn
 * down with `docker compose down -v` per dispatch anyway.
 */
import { randomUUID } from "node:crypto";
import { runId } from "./env.js";

/** RFC 4122 v4 UUID. Services that validate UUID-shaped IDs need real ones. */
export function uuid(): string {
	return randomUUID();
}

/**
 * A token unique to (dispatch, worker), safe to embed in a URL, a slug, a
 * Meilisearch document id or a Redis stream key.
 *
 * Deterministic in the worker index so a failure message points back at a
 * worker; random in the tail so a rerun against a still-warm stack never
 * inherits the previous run's rows.
 */
export function workerToken(workerIndex: number): string {
	const random = Math.floor(Math.random() * 0xffffff)
		.toString(16)
		.padStart(6, "0");
	return `${runId()}-w${workerIndex}-${random}`
		.toLowerCase()
		.replace(/[^a-z0-9-]/g, "");
}

/**
 * A token unique to a single test.
 *
 * Use this — not `workerToken` — whenever a test creates a row it will later
 * assert is absent, count, or delete: a worker-scoped token is shared by every
 * test that worker runs, so "exactly one match" becomes order-dependent.
 */
export function testToken(workerIndex: number, title: string): string {
	const slug = title
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-|-$/g, "")
		.slice(0, 40);
	return `${workerToken(workerIndex)}-${slug}`;
}

/** `YYYY-MM-DD` in UTC — for digest/partition keys that are date-scoped. */
export function todayUTC(): string {
	const iso = new Date().toISOString();
	// `toISOString` is always `YYYY-MM-DDTHH:MM:SS.sssZ`; index 10 is the `T`.
	return iso.slice(0, 10);
}

/** RFC 3339 timestamp in UTC, second precision — what the Go services parse. */
export function nowRFC3339(): string {
	return `${new Date().toISOString().slice(0, 19)}Z`;
}
