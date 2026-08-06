import { expect } from "@playwright/test";
import type { APIResponse } from "@playwright/test";

/**
 * Eventual consistency, asserted rather than slept through.
 *
 * A large part of Alt is asynchronous by construction: a Redis Streams
 * consumer ACKs only after a durable write (CLAUDE.md rule 10), a projector
 * rebuilds a read model from the event log, Meilisearch indexes on its own
 * schedule. The Hurl suites had exactly one tool for this — `--delay`, a flat
 * sleep before the next entry — which is the worst of both worlds: too short
 * and it flakes, too long and every run pays for the slowest case.
 *
 * `expect.poll` / `toPass` replace the sleep with the actual condition, so a
 * fast stack finishes in one interval and a slow one still passes. Nothing in
 * these suites should ever call `setTimeout` to wait for a side effect; if you
 * find yourself wanting to, the condition you are really waiting for belongs
 * here.
 */

/** Long enough for a stream reclaim cycle; short enough to fail a hung stack. */
const DEFAULT_TIMEOUT_MS = 30_000;

/**
 * Back off rather than hammer: a projector that takes 4s should not absorb 40
 * requests on the way there, and a 200ms first interval keeps the common
 * "already done" case instant.
 */
const DEFAULT_INTERVALS = [200, 400, 800, 1_500, 3_000] as const;

export type EventuallyOptions = {
	/** Overall budget. Default 30s. */
	readonly timeout?: number;
	/** Retry schedule in ms; the last value repeats. Default 200→3000. */
	readonly intervals?: readonly number[];
	/** Prose describing the condition — shown verbatim when the budget runs out. */
	readonly message?: string;
};

/**
 * Retries `body` until it passes or the budget runs out.
 *
 * Use for a *block* of assertions that must all hold at the same moment — a
 * projection that must have both the row and the correct counter, say.
 *
 *   await eventually(
 *     async () => {
 *       const items = await expectJsonStatus(await api.get("/v1/home"), 200, homeSchema);
 *       expect(items.map((i) => i.id)).toContain(seeded.id);
 *     },
 *     { message: "knowledge_home_items projection catches up with the appended event" },
 *   );
 */
export async function eventually(
	body: () => Promise<void> | void,
	options: EventuallyOptions = {},
): Promise<void> {
	const { timeout = DEFAULT_TIMEOUT_MS, intervals = DEFAULT_INTERVALS, message } = options;
	try {
		await expect(async () => {
			await body();
		}).toPass({ timeout, intervals: [...intervals] });
	} catch (error) {
		if (message === undefined) throw error;
		const detail = error instanceof Error ? error.message : String(error);
		throw new Error(`timed out waiting for: ${message}\n${detail}`);
	}
}

/**
 * Polls `probe` until its value satisfies the returned matcher.
 *
 * Use for a *single* value, where the assertion reads better as a matcher
 * chain than as a block:
 *
 *   await eventuallyValue(
 *     async () => (await api.get("/v1/jobs/" + id)).json(),
 *     "the recap job reaches a terminal state",
 *   ).toHaveProperty("status", "completed");
 */
export function eventuallyValue<T>(
	probe: () => Promise<T> | T,
	message: string,
	options: EventuallyOptions = {},
) {
	const { timeout = DEFAULT_TIMEOUT_MS, intervals = DEFAULT_INTERVALS } = options;
	return expect.poll(probe, { timeout, intervals: [...intervals], message });
}

/**
 * Polls an endpoint until it answers with one of `statuses`.
 *
 * The narrow, common case: a service that is up but whose dependency is still
 * settling answers 503 for a second or two. Asserting the eventual status is
 * honest; a fixed sleep before the request is not.
 */
export async function eventuallyStatus(
	request: () => Promise<APIResponse>,
	statuses: readonly number[],
	message: string,
	options: EventuallyOptions = {},
): Promise<APIResponse> {
	let last: APIResponse | undefined;
	await eventually(
		async () => {
			last = await request();
			expect(statuses).toContain(last.status());
		},
		{ ...options, message },
	);
	if (last === undefined) {
		throw new Error(`eventuallyStatus never issued a request: ${message}`);
	}
	return last;
}
