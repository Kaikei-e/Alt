import { compareEntryIds, expect, expectStreamAbsent, test } from "../src/fixtures.js";
import { callUnary } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { Procedure } from "../src/env.js";
import { articleCreatedEvent, publishBatchRequest, publishRequest } from "../src/events.js";
import { int64, streamInfoSchema } from "../src/schemas.js";

/**
 * `GetStreamInfo` — the port of `12-stream-info.hurl` and
 * `13-stream-info-not-found.hurl`.
 *
 * `12` is the scenario that made `--jobs 1` load-bearing. Its own comment says
 * so: "GetStreamInfo against alt:events:articles **after earlier files have
 * published events and created a consumer group**", and run.sh's says Hurl
 * 7.1's parallel default "races 12-stream-info ahead of 04/07 publishes +
 * 10 consumer-group create and yields HTTP 500 (XINFO on a stream that does
 * not exist yet)".
 *
 * Nothing about the RPC required that. It reads whatever key you name, and the
 * gateway lets you name any key, so each test here publishes into a stream it
 * owns and then reads that stream back. The assertion strengthens as a side
 * effect: `length` was `matches "^[1-9][0-9]*$"` — "some number of entries,
 * put there by somebody" — and is now an exact count of what this test wrote.
 */
test.describe("GetStreamInfo", () => {
	test("reports the entries and groups of a stream", { tag: "@contract" }, async ({
		api,
		stream,
		group,
	}) => {
		const events = [articleCreatedEvent(), articleCreatedEvent(), articleCreatedEvent()];
		await callUnary(api, Procedure.publishBatch, publishBatchRequest(stream, events));
		await callUnary(api, Procedure.createConsumerGroup, { stream, group, startId: "0" });

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);

		// Exact, where 12 could only assert "non-zero".
		expect(int64(info.length)).toBe(3);

		// Parity with 12's firstEntryId/lastEntryId regexes — now in the schema,
		// which also refuses the `""` the driver substitutes when Redis reports
		// no entry (redis_driver.go:218-226). "is a string" passed for that.
		expect(info.firstEntryId).toBeDefined();
		expect(info.lastEntryId).toBeDefined();

		// New: the two bound the same batch in the right order. A driver that
		// mapped LastEntry onto FirstEntryID would satisfy every regex above.
		expect(
			compareEntryIds(info.firstEntryId ?? "", info.lastEntryId ?? ""),
			"firstEntryId must precede lastEntryId",
		).toBeLessThan(0);

		// Parity with 12's `groups[*].name includes hurl-e2e-cg-{{run_id}}`.
		expect((info.groups ?? []).map((g) => g.name)).toContain(group);
	});

	test("reports a fresh group as idle", { tag: "@contract" }, async ({ api, stream, group }) => {
		// The counters inside ConsumerGroupInfo were never asserted at all —
		// 12 checked only `groups[*].name`. A group nobody has read from has no
		// consumers and nothing pending, and protojson omits both zeros, so
		// this pins the shape a consumer's own health check sees before it
		// starts: present, empty, and starting from the beginning.
		//
		// Publishing first matters: `pending` counts *delivered but unacked*
		// entries, so it stays 0 even with a backlog. A test that asserted
		// pending on an empty stream would pass for the wrong reason.
		await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent()));
		await callUnary(api, Procedure.createConsumerGroup, { stream, group, startId: "0" });

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		const mine = (info.groups ?? []).find((g) => g.name === group);
		expect(mine, `group ${group} should appear in XINFO GROUPS`).toBeDefined();
		expect(int64(mine?.consumers)).toBe(0);
		expect(int64(mine?.pending)).toBe(0);
		expect(mine?.lastDeliveredId).toBe("0-0");
	});

	test("reports a stream that has no groups", { tag: "@contract" }, async ({ api, stream }) => {
		// redis_driver.go:203-206 tolerates `no such key` from XINFO GROUPS and
		// returns an empty slice; protojson then omits the field entirely. So a
		// caller reading `groups` on a stream nobody consumes gets undefined,
		// not `[]` — the difference between `for (const g of info.groups)` and
		// a TypeError in a generated client. Uncovered before.
		await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent()));

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		expect(int64(info.length)).toBe(1);
		expect(info.groups ?? []).toHaveLength(0);
	});

	test("radix-tree counters are int64-as-string", { tag: "@contract" }, async ({ api, stream }) => {
		// 12's comment spelled out why this matters — "int64 fields are
		// serialized as JSON *strings* to preserve precision" — but only
		// asserted it for `length`. radixTreeKeys and radixTreeNodes have the
		// same encoding and are what an operator watches to size Redis memory.
		// The schema does the typing; this pins that they are populated at all,
		// since a stream with entries always has at least one radix-tree key.
		await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent()));

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		expect(int64(info.radixTreeKeys)).toBeGreaterThanOrEqual(1);
		expect(int64(info.radixTreeNodes)).toBeGreaterThanOrEqual(1);
	});

	test("errors on a stream that was never written", { tag: "@contract" }, async ({ api, stream }) => {
		// Parity with 13, which asserted `status >= 400` and `$.code exists` —
		// satisfied by any error at all, including a 503 from an unreachable
		// Redis, which is the opposite diagnosis. expectStreamAbsent pins both
		// halves and carries the reasoning; see src/fixtures.ts.
		await expectStreamAbsent(api, stream);
	});
});
