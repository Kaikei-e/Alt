import { test as base, expect } from "@playwright/test";
import type { APIRequestContext, APIResponse } from "@playwright/test";
import { callUnary, ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { testToken } from "../../_shared/ids.js";
import { env, Procedure } from "./env.js";

/**
 * Suite-wide fixtures.
 *
 * mq-hub has no authentication and no tenancy, so there is nothing to seed a
 * *session* with — the clients are cheap and worker-scoped. What each test
 * does need of its own is a **stream key**, and that is the fixture that
 * breaks the ordering the Hurl suite was built on.
 *
 * `e2e/hurl/mq-hub/run.sh` passed `--jobs 1` and called it load-bearing:
 * `12-stream-info.hurl` asserted `length >= 1` and `groups[*].name includes
 * hurl-e2e-cg-<run_id>` on `alt:events:articles`, which only held because
 * `04`, `07` and `10` had already run against that same shared key. Run them
 * in any other order and XINFO answered 500 on a stream that did not exist
 * yet.
 *
 * The fix is not a serial project — it is that nothing forced those scenarios
 * onto one key in the first place. `StreamGateway.Publish` only *warns* for an
 * unrecognised stream key (stream_gateway.go:31-36) and publishes anyway, so
 * every test can own a stream nothing else writes to. That turns "at least one
 * entry" into "exactly the three I published", which is a strictly stronger
 * assertion and an order-independent one.
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
	 * A Redis Stream key owned by exactly one test.
	 *
	 * Not one of the four `domain.StreamKey` constants on purpose — see the
	 * file comment. Deliberately prefixed `alt:events:` so it sorts with the
	 * real keys in `redis-cli --scan` when someone is debugging a failure with
	 * `KEEP_STACK=1`.
	 */
	stream: string;

	/** A consumer-group name owned by exactly one test. */
	group: string;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	api: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: { "Connect-Protocol-Version": "1" },
			});
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	bare: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.baseURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	stream: async ({}, use, testInfo) => {
		// testToken folds in RUN_ID, the worker index and a random tail, so two
		// shards on one daemon and a rerun against a still-warm Redis never
		// inherit each other's entries. No teardown: the slice is destroyed
		// with `docker compose down -v` per dispatch, and deleting the key here
		// would only race a sibling worker's XINFO.
		await use(`alt:events:e2e-${testToken(testInfo.workerIndex, testInfo.title)}`);
	},

	group: async ({}, use, testInfo) => {
		await use(`e2e-cg-${testToken(testInfo.workerIndex, testInfo.title)}`);
	},
});

export { expect };

/**
 * Asserts a procedure is mounted, for a procedure whose *empty* request is not
 * a safe probe.
 *
 * `_shared/connect.ts:expectProcedureMounted` always posts `{}`, which is
 * right for almost everything — but `GenerateTagsForArticle` with an empty
 * body takes the `timeoutMs <= 0` branch in generate_tags_usecase.go and
 * blocks on XREAD for the 60s default, well past the suite's 30s per-test
 * budget. So the probe has to be able to carry a request. Everything else is
 * the same claim: 404 means the handler was never registered, which is
 * CLAUDE.md rule 8 at the E2E boundary.
 */
export async function expectMountedWith(
	api: APIRequestContext,
	procedure: string,
	request: unknown,
	expected: readonly number[],
): Promise<APIResponse> {
	const response = await callUnary(api, procedure, request);
	expect(
		response.status(),
		`${procedure} answered ${response.status()}. 404 means the handler was never ` +
			`registered — check NewMQHubServiceHandler and the mux.Handle in main.go, ` +
			`not the request. Expected one of [${expected.join(", ")}].\n` +
			`body: ${(await response.text()).slice(0, 500)}`,
	).not.toBe(404);
	expect(expected, `${procedure} answered ${response.status()}`).toContain(response.status());
	return response;
}

/**
 * Asserts a stream key does not exist in Redis.
 *
 * The claim is spelled once, here, because two specs need it and it is the one
 * assertion in this suite inferred from the code rather than from an observed
 * response. The chain: `XINFO STREAM` on a missing key replies `ERR no such
 * key`; redis_driver.go:199 wraps it; handler.go:139 returns it **unwrapped by
 * `mapPublishErr`** — unlike every other RPC on this service — so connect-go
 * classifies it as `CodeUnknown`, which the protocol pairs with HTTP 500.
 *
 * That un-classified error is itself worth pinning. `GetStreamInfo` is the one
 * procedure whose "you asked about something that isn't there" answer is a 500
 * rather than a `not_found`, and a caller cannot retry-or-not on it. If this
 * test starts failing because the code became `not_found`, that is a fix and
 * this helper should follow it.
 */
export async function expectStreamAbsent(
	api: APIRequestContext,
	stream: string,
): Promise<void> {
	const error = await expectUnaryError(
		api,
		Procedure.getStreamInfo,
		{ stream },
		ConnectCode.unknown,
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
