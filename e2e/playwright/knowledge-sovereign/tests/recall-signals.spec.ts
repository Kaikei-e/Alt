import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { expectUnaryError } from "../../_shared/connect.js";
import { uuid } from "../../_shared/ids.js";
import { procedure } from "../src/env.js";
import { encodePayload, instant } from "../src/fixtures.js";
import { listRecallSignalsSchema } from "../src/schemas.js";

/**
 * Recall signals — the raw interaction stream `recall_candidate_view` scoring
 * is meant to draw on (`recall_signals`).
 *
 * Ports `13-recall-signal-append.hurl`, which asserted HTTP 200 on the write
 * and nothing else. `AppendRecallSignalResponse` has no fields, so 200 was
 * literally all Hurl could observe — a handler that parsed the request and
 * then dropped it on the floor would have passed. The read-back here is what
 * turns that into a test.
 */

test.describe("recall signals", () => {
	test(
		"AppendRecallSignal persists a signal that ListRecallSignals can read",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			const signalId = uuid();
			const itemKey = `article:${uuid()}`;
			const { occurredAt } = instant();
			const payload = { source: "playwright", dwell_ms: 4200 };

			const appended = await rpc.post(procedure("AppendRecallSignal"), {
				data: {
					signal: {
						signalId,
						userId: principal.userId,
						itemKey,
						signalType: "view",
						signalStrength: 1.0,
						occurredAt,
						payload: encodePayload(payload),
					},
				},
			});
			expect(appended.status()).toBe(200);
			expect(await appended.json()).toEqual({});

			const body = await expectJsonStatus(
				await rpc.post(procedure("ListRecallSignals"), {
					// `since_days` becomes `now() - N days` in the driver, so 0
					// would compare against `now()` and race the row that was
					// just written. 7 is the window the recall scoring itself
					// uses.
					data: { userId: principal.userId, sinceDays: 7 },
				}),
				200,
				listRecallSignalsSchema,
			);
			expect(body.signals).toHaveLength(1);
			const signal = body.signals[0];
			expect(signal?.signalId).toBe(signalId);
			expect(signal?.itemKey).toBe(itemKey);
			expect(signal?.signalType).toBe("view");
			expect(signal?.signalStrength).toBe(1);
			// The payload column is opaque to the service and is only ever read
			// back by the scorer, so a truncation or an encoding change here is
			// silent everywhere except a round trip.
			expect(signal?.payload).toBe(encodePayload(payload));
		},
	);

	test(
		"AppendRecallSignal requires occurred_at",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `occurred_at` is the only ordering the signal
			// stream has and the sole input to the `since_days` window — a
			// signal with no time is a signal that can never be scored, so the
			// handler refuses rather than stamping wall clock (which would also
			// make a replay non-deterministic).
			await expectUnaryError(
				rpc,
				procedure("AppendRecallSignal"),
				{
					signal: {
						signalId: uuid(),
						userId: principal.userId,
						itemKey: `article:${uuid()}`,
						signalType: "view",
						signalStrength: 1.0,
					},
				},
				"invalid_argument",
			);
		},
	);

	test(
		"ListRecallSignals is scoped to one user",
		{ tag: "@authz" },
		async ({ rpc, principal }) => {
			// New coverage. `WHERE user_id = $1` is the only scoping this read
			// has — there is no tenant predicate and no authentication on this
			// listener — so it is worth an explicit negative rather than an
			// inference from the positive test above.
			const { occurredAt } = instant();
			await rpc.post(procedure("AppendRecallSignal"), {
				data: {
					signal: {
						signalId: uuid(),
						userId: principal.userId,
						itemKey: `article:${uuid()}`,
						signalType: "view",
						signalStrength: 1.0,
						occurredAt,
					},
				},
			});

			const response = await rpc.post(procedure("ListRecallSignals"), {
				data: { userId: uuid(), sinceDays: 7 },
			});
			expect(response.status()).toBe(200);
			// protojson omits an empty repeated field, so "no signals" is `{}`.
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"the since_days window excludes older signals",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for the one parameter this RPC has. A window that
			// silently ignored `since_days` would make the recall scorer weigh
			// a year-old glance the same as this morning's read.
			const recentKey = `article:${uuid()}`;
			const staleKey = `article:${uuid()}`;
			const twoDaysAgo = instant(-2 * 24 * 60 * 60 * 1000);

			for (const [itemKey, at] of [
				[recentKey, instant().occurredAt],
				[staleKey, twoDaysAgo.occurredAt],
			] as const) {
				await rpc.post(procedure("AppendRecallSignal"), {
					data: {
						signal: {
							signalId: uuid(),
							userId: principal.userId,
							itemKey,
							signalType: "view",
							signalStrength: 0.5,
							occurredAt: at,
						},
					},
				});
			}

			const body = await expectJsonStatus(
				await rpc.post(procedure("ListRecallSignals"), {
					data: { userId: principal.userId, sinceDays: 1 },
				}),
				200,
				listRecallSignalsSchema,
			);
			const keys = body.signals.map((s) => s.itemKey);
			expect(keys).toContain(recentKey);
			expect(keys).not.toContain(staleKey);
		},
	);
});
