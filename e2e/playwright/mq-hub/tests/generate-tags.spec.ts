import { compareEntryIds, expect, test } from "../src/fixtures.js";
import {
	callUnary,
	ConnectCode,
	expectProcedureMounted,
	expectUnaryError,
} from "../../_shared/connect.js";
import { expectJson, expectJsonStatus } from "../../_shared/http.js";
import { CanonicalStream, Procedure } from "../src/env.js";
import { streamInfoSchema } from "../src/schemas.js";

/**
 * `GenerateTagsForArticle` — the port of `14-generate-tags-timeout.hurl`.
 *
 * This RPC is the one synchronous request/reply path in an otherwise
 * fire-and-forget service: it publishes a `TagGenerationRequested` event to
 * `alt:events:tags` with a `reply_to` pointing at a per-request stream, then
 * blocks on XREAD until a tag-generator answers or the timeout expires
 * (generate_tags_usecase.go:75-163).
 *
 * The mq-hub slice deliberately runs **no** tag-generator — that consumer is
 * only in the `tag-generator` compose profile — so the timeout branch is the
 * one this suite can reach. The happy path lives in the tag-generator suite,
 * which rounds a real article through this same RPC.
 */
test.describe("GenerateTagsForArticle", () => {
	/** Short enough to keep the suite fast, long enough not to race the XADD. */
	const TIMEOUT_MS = 200;

	function request(articleId: string): unknown {
		return {
			articleId,
			title: "Playwright timeout probe",
			content: "This article exists only to drive a call that must time out.",
			feedId: "e2e-feed",
			timeoutMs: TIMEOUT_MS,
		};
	}

	test("times out with deadline_exceeded when no consumer answers", { tag: "@contract" }, async ({
		api,
	}) => {
		// Parity with 14, with the code pinned. That file asserted only
		// `status >= 400 && status < 600` plus `message contains "timeout"`,
		// which cannot tell the timeout branch from the one right beside it:
		// handler.go:212-223 answers `deadline_exceeded` when
		// `errors.Is(err, domain.ErrReplyTimeout)` and `unavailable` otherwise,
		// and a caller's retry policy for the two is opposite — back off and
		// retry a timeout, page on a broker that is gone.
		//
		// That distinction is only load-bearing because the sentinel exists:
		// redis_driver.go:300 maps `redis.Nil` from the blocking XREAD onto
		// ErrReplyTimeout instead of letting it surface as a generic error.
		const error = await expectUnaryError(
			api,
			Procedure.generateTagsForArticle,
			request("e2e-timeout-1"),
			ConnectCode.deadlineExceeded,
		);
		expect(error.message).toBe("tag generation timeout");
	});

	test("honours the caller's timeout budget", { tag: "@contract" }, async ({ api }) => {
		// Parity with 14's `duration < 2000` on a 200ms request. The claim is
		// that `timeoutMs` reaches the XREAD BLOCK argument at all: the
		// alternative — the `timeoutMs <= 0` branch substituting
		// DefaultTagGenerationTimeoutMs — would hold the connection for 60
		// seconds, well past this suite's 30s per-test budget, and in
		// production would pin a goroutine and a Redis connection per caller.
		//
		// 2s of headroom over a 200ms budget: enough for a cold container and
		// a busy single-threaded Redis, tight enough that a 60s default fails.
		const started = Date.now();
		await expectUnaryError(
			api,
			Procedure.generateTagsForArticle,
			request("e2e-timeout-2"),
			ConnectCode.deadlineExceeded,
		);
		expect(Date.now() - started).toBeLessThan(2_000);
	});

	test("publishes the request event before waiting for a reply", { tag: "@contract" }, async ({ api }) => {
		// New. 14 asserted only that the call failed — which is also what a
		// handler that never published anything and just slept would do. The
		// request/reply pattern is worthless if the request half is missing:
		// tag-generator would never be asked, and every call would time out in
		// exactly this way.
		//
		// The observation is a *delta* on `lastEntryId`, not `length >= 1`.
		// `alt:events:tags` is the one stream this file cannot own — every test
		// in it drives the same RPC, which publishes to
		// domain.StreamKeyTags (generate_tags_usecase.go:131) — so under
		// `workers: 4` a non-empty stream proves only that somebody, sometime,
		// published. An entry ID that moved across this call proves an XADD
		// landed while it was running.
		//
		// What that does not prove is which call wrote it: a sibling test
		// publishing concurrently advances the same ID. That is acceptable
		// because the failure being caught is "this code path never XADDs at
		// all", which would leave the ID pinned for every worker at once.
		const before = await callUnary(api, Procedure.getStreamInfo, {
			stream: CanonicalStream.tags,
		});
		// The stream may not exist yet — this file's four tests are the only
		// writers, and GetStreamInfo answers 500 `no such key` until the first
		// of them publishes. That is a legitimate starting point, not a
		// failure, so it stands in for "no entries at all".
		const lastBefore =
			before.status() === 200
				? (await expectJson(before, streamInfoSchema)).lastEntryId
				: undefined;

		await expectUnaryError(
			api,
			Procedure.generateTagsForArticle,
			request("e2e-timeout-3"),
			ConnectCode.deadlineExceeded,
		);

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream: CanonicalStream.tags }),
			200,
			streamInfoSchema,
		);
		const lastAfter = info.lastEntryId;
		expect(
			lastAfter,
			`${CanonicalStream.tags} has no entries at all after a call that must have ` +
				`published a TagGenerationRequested event before blocking on its reply`,
		).toBeDefined();
		if (lastBefore !== undefined) {
			expect(
				compareEntryIds(lastBefore, lastAfter ?? ""),
				`${CanonicalStream.tags} lastEntryId did not move across the call ` +
					`(${lastBefore} → ${lastAfter}), so nothing was published — the RPC ` +
					`timed out without ever asking a tag-generator`,
			).toBeLessThan(0);
		}
	});

	test("is mounted even though the slice has no tag-generator", { tag: "@contract" }, async ({ api }) => {
		// handler.go:196 answers CodeUnimplemented when `generateTagsUsecase` is
		// nil — the shape a DI miss takes on this procedure, since
		// `NewHandlerWithGenerateTags` is one of two constructors and
		// `NewHandler` leaves the field nil. main.go:90 uses the former, so 501
		// here would mean the composition root regressed to the other one.
		//
		// The probe carries a request rather than the `{}` expectProcedureMounted
		// defaults to, because an empty body takes the `timeoutMs <= 0` branch
		// and blocks for the 60s default — well past this suite's 30s per-test
		// budget. That is what the helper's fourth parameter is for.
		await expectProcedureMounted(
			api,
			Procedure.generateTagsForArticle,
			// 504 is the only correct answer while nothing consumes
			// alt:events:tags: the procedure is registered, the usecase is
			// wired, and the reply never comes. 404 would be a missing mux
			// entry and 501 an unwired usecase — both wiring failures, both
			// excluded.
			[504],
			request("e2e-mounted"),
		);
	});
});
