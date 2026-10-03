import { test as base, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { testToken } from "../../_shared/ids.js";
import { env, Procedure, CanonicalStream, CanonicalGroup } from "./env.js";
import { assertCanonicalStreamsReset } from "./redis.js";

/**
 * Suite-wide fixtures.
 *
 * Provides worker-scoped Connect API clients and per-test Redis stream isolation.
 * When explicit E2E opt-in is enabled in CI, canonical streams are cleanly reset
 * before and after each test to ensure test independence regardless of execution order.
 */

export type WorkerFixtures = {
	/**
	 * The default Connect client.
	 *
	 * Carries `Connect-Protocol-Version: 1` because every Hurl entry sent it.
	 * mq-hub does *not* require it — main.go passes only
	 * `connect.WithInterceptors(...)`, never `WithRequireConnectProtocolHeader`
	 * — and tests/connect-surface.spec.ts asserts that separately, using
	 * `bare`. Keeping it on the default client means the rest of the suite
	 * exercises the same wire shape the previous suite did.
	 */
	api: APIRequestContext;

	/**
	 * A client with no default headers at all.
	 *
	 * Needed because Playwright's `extraHTTPHeaders` cannot be *removed* per
	 * request: proving "the Connect-Protocol-Version header is not required"
	 * or "an unsupported Content-Type is rejected" requires a context that
	 * never set them. Also used for the closed-port probe, where a header
	 * would be meaningless.
	 */
	bare: APIRequestContext;
};

export type TestFixtures = {
	/**
	 * Automatic per-test Redis stream isolation.
	 * In staging/CI with MQ_E2E_ALLOW_REDIS_RESET=1, cleans canonical streams
	 * before and after each test. Refuses/resets nothing for default localhost/user's running stack.
	 */
	resetIsolation: void;

	/**
	 * A canonical Redis Stream key (alt:events:articles) satisfying domain.StreamKey.IsValid().
	 */
	stream: string;

	/** A canonical consumer-group name (pre-processor-group) satisfying domain.ConsumerGroup.IsValid(). */
	group: string;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	api: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: { "Connect-Protocol-Version": "1", Authorization: `Bearer ${env.authToken}` },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	bare: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.baseURL, extraHTTPHeaders: { Authorization: `Bearer ${env.authToken}` } });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	resetIsolation: [
		async ({}, use) => {
			await assertCanonicalStreamsReset();
			try {
				await use();
			} finally {
				await assertCanonicalStreamsReset();
			}
		},
		{ auto: true },
	],

	stream: async ({}, use) => {
		await use(CanonicalStream.articles);
	},

	group: async ({}, use) => {
		await use(CanonicalGroup.preProcessor);
	},
});

export { expect };

/**
 * Asserts a stream key does not exist in Redis.
 *
 * `XINFO STREAM` on a missing key replies `ERR no such key`; redis_driver.go wraps
 * it, and handler.go maps it through `mapPublishErr` as `CodeUnavailable` (HTTP 503).
 */
export async function expectStreamAbsent(
	api: APIRequestContext,
	stream: string,
): Promise<void> {
	const error = await expectUnaryError(
		api,
		Procedure.getStreamInfo,
		{ stream },
		ConnectCode.unavailable,
	);
	expect(
		error.message ?? "",
		`GetStreamInfo(${stream}) failed, but not with Redis's "no such key" — ` +
			`this proves the RPC errored, not that the stream is absent`,
	).toContain("no such key");
}

/**
 * Compares two Redis Stream entry IDs numerically.
 *
 * `"1700000000000-0" < "999-0"` is true as strings and false as IDs, so the
 * monotonicity assertion cannot use a lexicographic compare.
 */
export function compareEntryIds(a: string, b: string): number {
	const [aMs = "0", aSeq = "0"] = a.split("-");
	const [bMs = "0", bSeq = "0"] = b.split("-");
	if (aMs !== bMs) return BigInt(aMs) < BigInt(bMs) ? -1 : 1;
	if (aSeq !== bSeq) return BigInt(aSeq) < BigInt(bSeq) ? -1 : 1;
	return 0;
}
