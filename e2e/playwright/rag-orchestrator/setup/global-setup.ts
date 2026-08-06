import { httpBody, waitForReady } from "../../_shared/readiness.js";
import type { Probe } from "../../_shared/readiness.js";
import { env } from "../src/env.js";
import { conversations, users } from "../src/seed.js";

/**
 * Readiness gate — the replacement for `00-readiness.hurl`.
 *
 * That file was a *test* whose whole body was `retry: 60 / retry-interval: 1s`,
 * and it only worked because the Hurl runner was pinned to `--jobs 1`. Under
 * `fullyParallel` there is no "run this one first": the gate has to sit outside
 * the suite. Probing here also collapses the failure mode — a stack that never
 * comes up fails **once**, naming the probe that never passed, instead of
 * failing forty tests with `ECONNREFUSED` and leaving the reader to work out
 * which dependency was actually missing.
 *
 * The probes run serially, in dependency order, because that is what makes the
 * first broken link the one reported:
 *
 *   Echo listener → rag-db pool → Connect listener → the seed is queryable
 *
 * The Hurl assertions themselves are not lost by moving them here: `/readyz`
 * and `/connect/health` are still asserted as real tests in
 * tests/health.spec.ts, where a regression in their *bodies* is a named
 * failure rather than a 90-second timeout.
 */

/** Generous: rag-db's first boot plus `atlas migrate apply` can take most of a minute. */
const READY = { timeout: 120_000, interval: 1_000 } as const;

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

export default async function globalSetup(): Promise<void> {
	await waitForReady(
		[
			/**
			 * Liveness first. `/healthz` is a static 200 that touches nothing
			 * (cmd/server/main.go:125), so it separates "the Echo listener is
			 * bound" from "the DB pool is usable" — which is the difference
			 * between a slow image pull and a broken migrator.
			 */
			httpBody(
				`${env.baseURL}/healthz`,
				(body) => isRecord(body) && body["status"] === "ok",
				"rag-orchestrator's Echo listener is up",
			),

			/**
			 * `/readyz` pings the pgx pool (main.go:128-134), so a green answer
			 * means rag-db is accepting connections *and* rag-db-migrator's
			 * `service_completed_successfully` gate has released. This is the
			 * race `00-readiness.hurl` spent its 60 retries on.
			 *
			 * Not `httpOk`: the 503 branch also returns JSON, and a suite that
			 * started against `{"status":"db down"}` would fail everywhere at
			 * once for a reason unrelated to the tests.
			 */
			httpBody(
				`${env.baseURL}/readyz`,
				(body) => isRecord(body) && body["status"] === "ready",
				"rag-orchestrator's DB pool answers a ping (rag-db + migrator are done)",
			),

			/**
			 * The Connect mux binds on a *different* server than Echo
			 * (main.go:171-191, two goroutines, two `http.Server`s). One being up
			 * says nothing about the other, so it gets its own probe.
			 */
			httpBody(
				`${env.connectURL}/connect/health`,
				(body) => isRecord(body) && body["status"] === "healthy",
				"rag-orchestrator's Connect-RPC listener is up",
			),

			seedVisible(),
		],
		READY,
	);
}

/**
 * Asserts the psql seed landed, **through the RPC that reads it**.
 *
 * `run.sh` pipes `setup/db-seed.sql` into the rag-db container between
 * `compose up --wait` and the test run. If that step silently no-ops — a
 * migration renamed a column, `ON CONFLICT` swallowed a genuine constraint
 * failure, the wrong database name — every history, pagination and delete test
 * fails with its own confusing message. Establishing the pre-state once, here,
 * turns that into a single legible failure.
 *
 * It also buys the one thing the destructive test cannot assert for itself:
 * that the conversation it deletes existed beforehand. Asserting pre-existence
 * inside that test would make it fail on a CI retry (the row is gone by then,
 * legitimately), so the fact is established before any test runs.
 */
function seedVisible(): Probe {
	const url = `${env.connectURL}/alt.augur.v2.AugurService/ListConversations`;
	const expected = [
		{ user: users.history, label: "the history conversation", ids: [conversations.history] },
		{
			user: users.paging,
			label: "the three paging conversations",
			ids: [conversations.paging1, conversations.paging2, conversations.paging3],
		},
		{
			user: users.deletableOwner,
			label: "the deletable conversation",
			ids: [conversations.deletable],
		},
		{
			user: users.protectedOwner,
			label: "the protected conversation",
			ids: [conversations.protected],
		},
	] as const;

	return {
		label: `setup/db-seed.sql is visible through AugurService/ListConversations`,
		run: async (api) => {
			for (const { user, label, ids } of expected) {
				const response = await api.post(url, {
					headers: {
						"Content-Type": "application/json",
						"Connect-Protocol-Version": "1",
						"X-Alt-User-Id": user,
					},
					data: {},
					timeout: 10_000,
				});
				if (!response.ok()) {
					throw new Error(
						`ListConversations for ${user} answered ${response.status()}: ` +
							`${(await response.text()).slice(0, 300)}`,
					);
				}
				const body: unknown = await response.json();
				const rows = isRecord(body) ? body["conversations"] : undefined;
				const got = (Array.isArray(rows) ? rows : [])
					.map((row) => (isRecord(row) ? row["id"] : undefined))
					.filter((id): id is string => typeof id === "string")
					.sort();
				const want = [...ids].sort();
				if (got.length !== want.length || got.some((id, i) => id !== want[i])) {
					throw new Error(
						`expected ${label} for ${user}: [${want.join(", ")}], saw ` +
							`[${got.join(", ")}]. The psql seed in run.sh did not apply as ` +
							`written — check the "seeding augur fixtures" step's output and ` +
							`rag-db-migrator's log.`,
					);
				}
			}
		},
	};
}
