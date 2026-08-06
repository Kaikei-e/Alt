import { compareEntryIds, expect, test } from "../src/fixtures.js";
import { callUnary, ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { CanonicalStream, Procedure } from "../src/env.js";
import { articleCreatedEvent, publishRequest } from "../src/events.js";
import { int64, publishResponseSchema, streamInfoSchema } from "../src/schemas.js";

/**
 * Single-event `Publish` — the port of `04-publish-happy.hurl`,
 * `05-publish-validation.hurl` and `06-publish-nil-event.hurl`, plus the three
 * validation branches those three never reached.
 *
 * `domain.Event.Validate` has four required fields (event.go:81-94) and the
 * Hurl suite exercised exactly one of them. The other three are not
 * theoretical: `event_id` is the field the proto documents as the consumer's
 * dedupe key under at-least-once delivery, so an event accepted without one is
 * an event no downstream service can deduplicate.
 */
test.describe("Publish", () => {
	test("publishes a valid event and returns its stream ID", { tag: "@smoke" }, async ({ api, stream }) => {
		const body = await expectJsonStatus(
			await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
			publishResponseSchema,
		);
		// Parity with 04-publish-happy.hurl: `success == true` and a messageId
		// matching `^[0-9]+-[0-9]+$`. Both live in publishResponseSchema now, so
		// a response that dropped `success` — rather than setting it false —
		// fails here too. The Hurl captured `messageId` "for audit logging" and
		// no later file ever read it.
		expect(body.messageId).toMatch(/^[0-9]+-[0-9]+$/);
	});

	test("publishes to a canonical stream key", { tag: "@smoke" }, async ({ api }) => {
		// The rest of this suite publishes to per-test keys, which take the
		// `!stream.IsValid()` warning branch of stream_gateway.go:32. This is
		// the control for that: the four `domain.StreamKey` constants are the
		// ones production actually uses, and they must keep working. Only the
		// response is asserted — the stream itself is shared with every other
		// worker, so nothing here may assert its length.
		await expectJsonStatus(
			await callUnary(
				api,
				Procedure.publish,
				publishRequest(CanonicalStream.articles, articleCreatedEvent()),
			),
			200,
			publishResponseSchema,
		);
	});

	test("assigns strictly increasing stream IDs", { tag: "@contract" }, async ({ api, stream }) => {
		// The proto's delivery contract is at-least-once with `event_id` as the
		// dedupe key, but consumers also checkpoint by *stream* ID — an
		// XAUTOCLAIM reclaim loop (CLAUDE.md rule 10) resumes from the last one
		// it saw. If two XADDs to one key could hand back the same ID, or a
		// smaller one, a consumer would silently skip everything between. The
		// comparison is numeric: "1700000000000-0" sorts before "999-0" as a
		// string.
		const first = await expectJsonStatus(
			await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
			publishResponseSchema,
		);
		const second = await expectJsonStatus(
			await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
			publishResponseSchema,
		);
		expect(compareEntryIds(first.messageId, second.messageId)).toBeLessThan(0);
	});

	test("a published event is visible in the stream it names", { tag: "@contract" }, async ({
		api,
		stream,
	}) => {
		// The Hurl suite asserted the *response* of a publish and, separately,
		// that a long-lived shared stream was non-empty. It never connected the
		// two, so a handler that answered `{"success":true,"messageId":"1-0"}`
		// without writing anything would have passed. Publishing into a stream
		// this test owns makes the count exact rather than "at least one".
		//
		// No `eventually` here on purpose: XADD is synchronous, so the entry is
		// durable before the RPC returns. Polling would imply an asynchrony
		// that does not exist.
		await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent()));
		await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent()));

		const info = await expectJsonStatus(
			await callUnary(api, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		expect(int64(info.length)).toBe(2);
	});

	test("rejects an event with no event_type", { tag: "@contract" }, async ({ api, stream }) => {
		// Parity with 05-publish-validation.hurl, strengthened twice over. That
		// file asserted only `status >= 400 && status < 600` with a comment
		// saying "we don't pin the exact Connect code" — but the code is pinned
		// by handler.go's `mapPublishErr`, which exists precisely so a caller
		// can tell bad input (invalid_argument) from an unavailable Redis
		// (unavailable). A band that accepts both accepts the regression that
		// classifier was written to prevent.
		const error = await expectUnaryError(
			api,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent({ eventType: "" })),
			ConnectCode.invalidArgument,
		);
		expect(error.message).toContain("event_type");
	});

	test("rejects an event with no event_id", { tag: "@contract" }, async ({ api, stream }) => {
		// event.go:82. Never covered. `event_id` is the dedupe key every
		// consumer is required to use, so accepting an event without one would
		// make at-least-once delivery unimplementable downstream.
		const error = await expectUnaryError(
			api,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent({ eventId: "" })),
			ConnectCode.invalidArgument,
		);
		expect(error.message).toContain("event_id");
	});

	test("rejects an event with no source", { tag: "@contract" }, async ({ api, stream }) => {
		// event.go:88. Never covered. `source` is what makes a stream entry
		// attributable when several producers write the same event type.
		const error = await expectUnaryError(
			api,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent({ source: "" })),
			ConnectCode.invalidArgument,
		);
		expect(error.message).toContain("source");
	});

	test("substitutes a created_at when the caller omits it", { tag: "@contract" }, async ({
		api,
		stream,
	}) => {
		// `Validate` also requires created_at (event.go:91), but that branch is
		// unreachable from the wire: handler.go:179-182 substitutes
		// `time.Now()` whenever the Timestamp is absent. Asserting the
		// *success* documents which of the four required fields is actually
		// enforced at the RPC boundary and which is filled in for you — the
		// kind of thing a reader would otherwise have to infer from two files.
		const request = publishRequest(stream, articleCreatedEvent()) as {
			event: Record<string, unknown>;
		};
		delete request.event["createdAt"];

		await expectJsonStatus(
			await callUnary(api, Procedure.publish, request),
			200,
			publishResponseSchema,
		);
	});

	test("rejects a request with no event at all", { tag: "@contract" }, async ({ api, stream }) => {
		// Parity with 06-publish-nil-event.hurl, which already pinned
		// `code == "invalid_argument"`. expectUnaryError additionally requires
		// the HTTP status the Connect protocol pairs with that code (400): a
		// handler answering the right envelope under the wrong status breaks
		// every generated client, and vice versa.
		//
		// This is the one branch that short-circuits in handler.go:39 before
		// the gateway, which is why its message differs from the three above.
		const error = await expectUnaryError(
			api,
			Procedure.publish,
			{ stream },
			ConnectCode.invalidArgument,
		);
		expect(error.message).toContain("event is required");
	});

	test("accepts a publish from an anonymous caller", { tag: "@authz" }, async ({ bare, stream }) => {
		// Pins the *current* posture, deliberately. mq-hub mounts no
		// authentication: main.go's only handler option is the logging
		// interceptor, and `middleware.PeerIdentityMiddleware` is unwired dead
		// code that main.go announces at startup with `peer_identity_disabled`
		// (CLAUDE.md rule 8 — a disabled control must be loud, not inferred).
		// Anyone who can reach :9500 can write to any stream.
		//
		// When an mTLS listener does arrive, this test fails — which is the
		// intended signal, not a regression. See tests/topology.spec.ts.
		await expectJsonStatus(
			await callUnary(bare, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
			publishResponseSchema,
		);
	});
});
