import { test as base } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { uuid } from "../../_shared/ids.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { env, procedure } from "./env.js";
import { appendEventSchema } from "./schemas.js";

/**
 * Suite-wide fixtures.
 *
 * The clients are worker-scoped — they hold no per-test state and creating an
 * `APIRequestContext` per test would buy nothing. The **identities** are
 * test-scoped, and that is the whole isolation story here.
 *
 * The Hurl suite minted one `tenant_id` / `user_id` / `lens_id` / `event_id`
 * set in `run.sh` and threaded it through all twenty scenarios with
 * `--variable`, which is exactly why it needed `--jobs 1`: scenario 05
 * asserted "`ListKnowledgeEvents` returns exactly one event" for a pair that
 * scenario 03 had written to, and scenario 11 asserted "`ListLenses` returns
 * exactly one lens" for a user scenario 09 had created one for. Any
 * concurrency at all breaks both counts.
 *
 * Giving every test its own `userId` makes those same cardinality assertions
 * true *by construction* rather than by ordering: every read in this suite is
 * scoped by a user nothing else in the run has ever written to, so "exactly
 * one" is a real claim about the handler and not a claim about the schedule.
 * `tenantId` is worker-scoped rather than test-scoped on purpose — it keeps
 * the cross-tenant negative in tests/events.spec.ts meaningful (a *different*
 * tenant genuinely has rows) without making any positive read ambiguous,
 * because the driver's predicate is `tenant_id = $2 AND (user_id = $3 OR
 * user_id IS NULL)` and the user half is always unique.
 */

export type Principal = {
	/** Shared with the other tests this worker runs. */
	readonly tenantId: string;
	/** Unique to this test. Every read in the test is scoped by it. */
	readonly userId: string;
	/** Unique to this test; the prefix for every dedupe_key it writes. */
	readonly token: string;
};

type WorkerFixtures = {
	/** Connect-RPC JSON on :9500. */
	rpc: APIRequestContext;
	/** Operator surface on :9501 — /health, /metrics, /admin/*. */
	admin: APIRequestContext;
	/** One tenant per worker; see the note above on why it is not per-test. */
	tenantId: string;
};

type TestFixtures = {
	principal: Principal;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	tenantId: [
		async ({}, use) => {
			await use(uuid());
		},
		{ scope: "worker" },
	],

	rpc: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: {
					"Content-Type": "application/json",
					// Required by the Connect protocol for unary JSON. connect-go
					// tolerates its absence, but sending it is what makes these
					// requests the same shape the generated clients emit — and a
					// server that started *rejecting* it would be a real break.
					"Connect-Protocol-Version": "1",
				},
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	admin: [
		async ({ playwright }, use) => {
			// No Authorization header on purpose: the staging slice sets
			// ADMIN_AUTH=disabled (compose.staging.yaml), and
			// tests/topology.spec.ts asserts that posture explicitly rather
			// than leaving it implied by every admin call that happens to work.
			const context = await playwright.request.newContext({ baseURL: env.metricsURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	principal: async ({ tenantId }, use, testInfo) => {
		const slug = testInfo.title
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, "-")
			.replace(/^-|-$/g, "")
			.slice(0, 40);
		await use({
			tenantId,
			userId: uuid(),
			token: `pw-${env.runId}-w${testInfo.workerIndex}-${slug}-${uuid().slice(0, 8)}`,
		});
	},
});

export { expect } from "@playwright/test";

// ---------------------------------------------------------------------------
// Seeding helpers
// ---------------------------------------------------------------------------

/** proto `bytes` fields travel as base64 in proto3-JSON. */
export function encodePayload(payload: unknown): string {
	return Buffer.from(JSON.stringify(payload), "utf8").toString("base64");
}

/**
 * A single instant rendered both ways.
 *
 * `GetTodayDigest` takes a `YYYY-MM-DD` date and the projector derives the
 * digest's `digest_date` from the event's own `occurred_at`
 * (`occurredAt.Format(time.DateOnly)` in foldArticleCreated). Computing the
 * two from separate `new Date()` calls would disagree for a few milliseconds
 * a day, at UTC midnight — a flake that reproduces once per run at 00:00Z and
 * never in review.
 */
export function instant(offsetMs = 0): { readonly occurredAt: string; readonly date: string } {
	const iso = new Date(Date.now() + offsetMs).toISOString();
	// `toISOString` is always `YYYY-MM-DDTHH:MM:SS.sssZ`; the Go handlers parse
	// RFC 3339 and second precision is what the retired suite sent.
	return { occurredAt: `${iso.slice(0, 19)}Z`, date: iso.slice(0, 10) };
}

export type AppendEventInput = {
	readonly tenantId: string;
	readonly userId?: string;
	readonly eventType: string;
	readonly aggregateType?: string;
	readonly aggregateId?: string;
	readonly dedupeKey: string;
	readonly occurredAt: string;
	readonly payload?: unknown;
	readonly eventId?: string;
	readonly actorType?: string;
	readonly actorId?: string;
};

/** Builds the proto3-JSON body for `AppendKnowledgeEvent`. */
export function appendEventBody(input: AppendEventInput): Record<string, unknown> {
	const event: Record<string, unknown> = {
		eventId: input.eventId ?? uuid(),
		occurredAt: input.occurredAt,
		tenantId: input.tenantId,
		actorType: input.actorType ?? "user",
		actorId: input.actorId ?? "e2e-playwright",
		eventType: input.eventType,
		aggregateType: input.aggregateType ?? "article",
		aggregateId: input.aggregateId ?? "",
		dedupeKey: input.dedupeKey,
	};
	if (input.userId !== undefined) event["userId"] = input.userId;
	// `payload` is always sent, defaulting to an empty object.
	//
	// `knowledge_events.payload` is NOT NULL, so omitting the field made the
	// insert fail with a Postgres constraint violation surfaced as a Connect
	// `internal` — and because `appendEvent` is a *precondition* for most of
	// this suite, one missing default failed seventeen tests in places far from
	// the cause. A seeding helper must produce a valid event; a test that wants
	// to know what the service does with a malformed one should say so at the
	// call site.
	event["payload"] = encodePayload(input.payload ?? {});
	return { event };
}

/**
 * Appends one event and returns its `event_seq`.
 *
 * Throws rather than returning a status, because every caller uses this as a
 * *precondition*: a seeding failure reported as an assertion three steps later
 * ("the projection is empty") sends the reader after the projector when the
 * append is what broke.
 */
export async function appendEvent(
	rpc: APIRequestContext,
	input: AppendEventInput,
): Promise<string> {
	const response = await rpc.post(procedure("AppendKnowledgeEvent"), {
		data: appendEventBody(input),
	});
	const body = await expectJsonStatus(response, 200, appendEventSchema);
	return body.eventSeq;
}
