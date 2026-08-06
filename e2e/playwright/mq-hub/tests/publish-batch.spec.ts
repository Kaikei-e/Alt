import { expect, expectStreamAbsent, test } from "../src/fixtures.js";
import { callUnary, ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { MAX_BATCH_SIZE, Procedure } from "../src/env.js";
import {
	articleCreatedEvent,
	batchEvents,
	publishBatchRequest,
} from "../src/events.js";
import { int64, publishBatchResponseSchema, streamInfoSchema } from "../src/schemas.js";

/**
 * `PublishBatch` — the port of `07-publish-batch-happy.hurl`,
 * `08-publish-batch-invalid.hurl` and `09-publish-batch-oversize.hurl`, plus
 * the boundary and the persistence claim those three left unasserted.
 *
 * Two things change in the port. First, the 1001-event fixture is built here
 * rather than by `e2e/fixtures/mq-hub/gen-batch-oversize.py`, so run.sh no
 * longer needs Python and the file no longer has to be gitignored and
 * regenerated. Second, `08`'s README entry claimed "**no** partial
 * persistence — gateway validates all events before hitting Redis" while the
 * executable asserted only the error envelope; publishing into a stream this
 * test owns lets that claim actually be checked.
 */
test.describe("PublishBatch", () => {
	test("publishes every event in a valid batch", { tag: "@smoke" }, async ({ api, stream }) => {
		const events = [articleCreatedEvent(), articleCreatedEvent(), articleCreatedEvent()];
		const body = await expectJsonStatus(
			await callUnary(api, Procedure.publishBatch, publishBatchRequest(stream, events)),
			200,
			publishBatchResponseSchema,
		);

		// Parity with 07: successCount == 3, messageIds count == 3, each
		// matching the Redis entry-ID regex (now in the schema, applied to
		// every element rather than to indices 0, 1 and 2 by hand).
		expect(body.successCount).toBe(3);
		expect(body.messageIds ?? []).toHaveLength(3);

		// Parity with 07's `failureCount not exists`, which reads as a quirk
		// until you know why: protojson omits zero values, so the *absence* of
		// the field is how "nothing failed" is spelled on the wire. A client
		// reading `failureCount` without a default sees undefined, not 0.
		expect(body.failureCount).toBeUndefined();
		expect(body.errors).toBeUndefined();

		// New: the IDs are distinct. A pipeline that reused one XAdd command,
		// or a driver that returned `cmds[0].Val()` for every index, would still
		// satisfy every assertion above.
		expect(new Set(body.messageIds ?? []).size).toBe(3);
	});

	test("the batch actually lands in the stream", { tag: "@contract" }, async ({ api, stream }) => {
		// 07 asserted the response and stopped. Reading the stream back proves
		// the pipeline executed rather than the handler counting its input —
		// and it can be exact, not "at least", because nothing else writes this
		// key. `driver.PublishBatch` returns one messageID slot per input event
		// (redis_driver.go:118-181), so length must equal the batch size.
		await callUnary(api, Procedure.publishBatch, publishBatchRequest(stream, batchEvents(5)));

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		expect(int64(info.length)).toBe(5);
	});

	test("accepts an empty batch as a no-op", { tag: "@contract" }, async ({ api, stream }) => {
		// redis_driver.go:126 early-returns `[]string{}, nil` for a zero-length
		// batch, so this succeeds with every field at its zero value — which
		// protojson omits, making the whole response `{}`. Uncovered before,
		// and it matters: a producer that batches on a timer will send an empty
		// flush eventually, and the alternative answer (invalid_argument) would
		// make that a paging error.
		const body = await expectJsonStatus(
			await callUnary(api, Procedure.publishBatch, publishBatchRequest(stream, [])),
			200,
			publishBatchResponseSchema,
		);
		expect(body.successCount ?? 0).toBe(0);
		expect(body.messageIds ?? []).toHaveLength(0);

		// And it creates nothing: no XADD ran, so the key does not exist.
		await expectStreamAbsent(api, stream);
	});

	test("rejects the whole batch when one event is invalid", { tag: "@contract" }, async ({
		api,
		stream,
	}) => {
		// Parity with 08, with the code pinned. That file accepted any 4xx or
		// 5xx, which cannot distinguish "your batch is malformed" from "Redis
		// is down" — the two answers `mapPublishErr` exists to separate. The
		// invalid event is last, as in the original fixture, so a gateway that
		// validated lazily *while* publishing would have written the first two
		// before failing.
		const events = [
			articleCreatedEvent(),
			articleCreatedEvent(),
			articleCreatedEvent({ eventType: "" }),
		];
		const error = await expectUnaryError(
			api,
			Procedure.publishBatch,
			publishBatchRequest(stream, events),
			ConnectCode.invalidArgument,
		);
		expect(error.message).toContain("event_type");

		// The claim 08's README made and never checked: stream_gateway.go:59-67
		// validates every event before calling the driver, so nothing at all
		// was written — not two of three. On a shared stream this is
		// unassertable, which is exactly why the Hurl file did not assert it.
		await expectStreamAbsent(api, stream);
	});

	test("rejects a batch of exactly MAX_BATCH_SIZE + 1", { tag: ["@contract", "@slow"] }, async ({
		api,
		stream,
	}) => {
		// Parity with 09, minus the Python generator. publish_usecase.go:103
		// compares `batchSize > u.maxBatchSize`, so 1001 is the first rejected
		// size and the error is classified invalid_argument by mapPublishErr's
		// `errors.Is(err, usecase.ErrBatchTooLarge)` arm.
		const error = await expectUnaryError(
			api,
			Procedure.publishBatch,
			publishBatchRequest(stream, batchEvents(MAX_BATCH_SIZE + 1)),
			ConnectCode.invalidArgument,
		);
		expect(error.message).toContain("batch size exceeds");

		// Rejected *before* touching Redis (the guard is the first statement of
		// PublishBatch), so the stream is never created. Never asserted before.
		await expectStreamAbsent(api, stream);
	});

	test("accepts a batch of exactly MAX_BATCH_SIZE", { tag: ["@contract", "@slow"] }, async ({ api, stream }) => {
		// The other side of the boundary, and the half that catches an
		// off-by-one. `>` versus `>=` in publish_usecase.go:103 is a one-
		// character change that no test in the Hurl suite could have detected:
		// it only ever sent 3 events or 1001. A producer batching at exactly
		// the documented limit would then start failing in production.
		const body = await expectJsonStatus(
			await callUnary(
				api,
				Procedure.publishBatch,
				publishBatchRequest(stream, batchEvents(MAX_BATCH_SIZE)),
			),
			200,
			publishBatchResponseSchema,
		);
		expect(body.successCount).toBe(MAX_BATCH_SIZE);
		expect(body.messageIds ?? []).toHaveLength(MAX_BATCH_SIZE);
	});
});
