import { expect, expectStreamAbsent, test } from "../src/fixtures.js";
import { callUnary, ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { Procedure } from "../src/env.js";
import { articleCreatedEvent, publishRequest } from "../src/events.js";
import {
	createConsumerGroupResponseSchema,
	int64,
	streamInfoSchema,
} from "../src/schemas.js";

/**
 * `CreateConsumerGroup` — the port of `10-consumer-group-create.hurl` and
 * `11-consumer-group-idempotent.hurl`.
 *
 * Those two were the clearest case of the ordering the Hurl runner had to
 * enforce with `--jobs 1`: 11 asserted idempotency by re-sending 10's request,
 * so running 11 first would have created the group and tested nothing, and
 * running them on different workers would have raced. Here the idempotency
 * test creates the group itself and then re-creates it — the same claim,
 * seeded by the test that makes it.
 *
 * Idempotency is not cosmetic here. Every Alt stream consumer calls
 * CreateConsumerGroup on startup (that is what `ConsumerGroupPreProcessor` and
 * friends in domain/stream.go are for), so a service restart or a second
 * replica must not fail on BUSYGROUP.
 */
test.describe("CreateConsumerGroup", () => {
	test("creates a group on a stream", { tag: "@smoke" }, async ({ api, stream, group }) => {
		const body = await expectJsonStatus(
			await callUnary(api, Procedure.createConsumerGroup, { stream, group, startId: "0" }),
			200,
			createConsumerGroupResponseSchema,
		);
		// Parity with 10, strengthened: that file asserted `message isString`,
		// which the failure branch's "failed to create consumer group" also
		// satisfies. The schema pins the literal.
		expect(body.success).toBe(true);
	});

	test("re-creating the same group succeeds", { tag: "@contract" }, async ({ api, stream, group }) => {
		// Parity with 11. redis_driver.go:187 swallows Redis's BUSYGROUP reply
		// and returns nil, which is what makes "create the group on startup"
		// safe to run on every boot and in every replica. Self-seeded, so it no
		// longer depends on a previous file having run.
		const request = { stream, group, startId: "0" };
		await expectJsonStatus(
			await callUnary(api, Procedure.createConsumerGroup, request),
			200,
			createConsumerGroupResponseSchema,
		);
		await expectJsonStatus(
			await callUnary(api, Procedure.createConsumerGroup, request),
			200,
			createConsumerGroupResponseSchema,
		);
	});

	test("MKSTREAM creates the stream when it does not exist", { tag: "@contract" }, async ({
		api,
		stream,
		group,
	}) => {
		// The driver calls XGroupCreateMkStream, not XGroupCreate — so a
		// consumer that boots before its producer has ever published still gets
		// its group, on an empty stream. Never covered: the Hurl suite only
		// ever created a group on `alt:events:articles`, which earlier
		// scenarios had already populated.
		await expectStreamAbsent(api, stream);

		await expectJsonStatus(
			await callUnary(api, Procedure.createConsumerGroup, { stream, group, startId: "0" }),
			200,
			createConsumerGroupResponseSchema,
		);

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		// The stream now exists but is empty — protojson omits the zero length
		// and both empty entry IDs, which is the shape a consumer sees on a
		// cold system and must not mistake for an error.
		expect(int64(info.length)).toBe(0);
		expect(info.firstEntryId).toBeUndefined();
		expect(info.lastEntryId).toBeUndefined();
		expect((info.groups ?? []).map((g) => g.name)).toContain(group);
	});

	test('startId "$" starts the group at the stream tail', { tag: "@contract" }, async ({
		api,
		stream,
		group,
	}) => {
		// The proto documents both start IDs ("0" for beginning, "$" for new
		// messages only) and the Hurl suite exercised only "0". The difference
		// is visible in `lastDeliveredId`: a group created at "$" after two
		// entries reports the second entry's ID as already delivered, so a
		// consumer joining it will not replay the backlog. That is the entire
		// semantic difference between the two modes, and it was untested.
		await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent()));
		const second = await callUnary(
			api,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent()),
		);
		const lastPublished = ((await second.json()) as { messageId: string }).messageId;

		await expectJsonStatus(
			await callUnary(api, Procedure.createConsumerGroup, { stream, group, startId: "$" }),
			200,
			createConsumerGroupResponseSchema,
		);

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		const mine = (info.groups ?? []).find((g) => g.name === group);
		expect(mine, `group ${group} should appear in XINFO GROUPS`).toBeDefined();
		expect(mine?.lastDeliveredId).toBe(lastPublished);
	});

	test("rejects an unparseable start ID", { tag: "@contract" }, async ({ api, stream, group }) => {
		// Redis answers `ERR Invalid stream ID specified as stream command
		// argument`; the driver wraps it, and handler.go:126 turns anything
		// that is not BUSYGROUP into CodeUnavailable. Two things worth pinning:
		// the code (a caller must not retry a malformed request forever), and
		// that the body carries the handler's fixed "failed to create consumer
		// group" rather than Redis's text — the handler logs the detail and
		// deliberately does not return it.
		const error = await expectUnaryError(
			api,
			Procedure.createConsumerGroup,
			{ stream, group, startId: "not-a-stream-id" },
			ConnectCode.unavailable,
		);
		expect(error.message).toBe("failed to create consumer group");
		expect(
			error.message,
			"the Redis error text must not reach the caller (handler.go:121-127 logs it instead)",
		).not.toContain("ERR");
	});
});
