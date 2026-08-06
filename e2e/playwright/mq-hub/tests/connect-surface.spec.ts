import { expect, test } from "../src/fixtures.js";
import {
	callUnary,
	expectProcedureMounted,
	procedurePath,
} from "../../_shared/connect.js";
import { expectJson, expectMethodNotAllowed, expectStatus } from "../../_shared/http.js";
import { MQHUB_SERVICE, Procedure } from "../src/env.js";
import { connectErrorSchema } from "../src/schemas.js";

/**
 * The Connect-RPC mux itself — new coverage.
 *
 * `MQHubService` declares six procedures (proto/services/mqhub/v1/mqhub.proto)
 * and `NewMQHubServiceHandler` routes them through a hand-written switch whose
 * default arm is `http.NotFound` (mqhub.connect.go:222-240). The Hurl suite
 * called five of the six — but always with a business assertion, never with a
 * registration one, so it could not have distinguished "this procedure answers
 * an error" from "this path does not exist". Those look identical to any
 * assertion of the form "not 2xx", and the second is what a `mux.Handle` line
 * lost in a refactor, or a handler skipped because its DI dependency came back
 * nil, actually looks like.
 *
 * This is CLAUDE.md rule 8 at the E2E boundary. The discriminator is 404 vs
 * anything else: mq-hub has no auth interceptor, so a mounted procedure
 * answers a *business* status to an empty request rather than the 401 that
 * plays this role in alt-backend's suite.
 */

/**
 * Every procedure whose empty-request answer is safe to probe, with the status
 * that answer is and why.
 *
 * `GenerateTagsForArticle` is absent on purpose: with no `timeoutMs` it takes
 * generate_tags_usecase.go's 60s default and blocks. Its registration is
 * asserted in tests/generate-tags.spec.ts, with a request.
 */
const MOUNTED = [
	{
		name: "Publish",
		procedure: Procedure.publish,
		// handler.go:39 short-circuits on a nil event with
		// CodeInvalidArgument, which the Connect protocol pairs with 400.
		expected: [400],
	},
	{
		name: "PublishBatch",
		procedure: Procedure.publishBatch,
		// An absent `events` decodes to an empty slice, and
		// redis_driver.go:126 early-returns success for a zero-length batch.
		expected: [200],
	},
	{
		name: "CreateConsumerGroup",
		procedure: Procedure.createConsumerGroup,
		// The empty `startId` is not a valid stream ID, so Redis answers `ERR
		// Invalid stream ID specified as stream command argument` and
		// handler.go:126 maps anything that is not BUSYGROUP to CodeUnavailable
		// (503). Exactly 503, not a band: 200 would mean `XGROUP CREATE "" ""
		// "" MKSTREAM` had been accepted and the empty-string key created,
		// which is the state the sibling "GetStreamInfo is mounted" asserts is
		// impossible by expecting 500 (`no such key`) on that same key.
		expected: [503],
	},
	{
		name: "GetStreamInfo",
		procedure: Procedure.getStreamInfo,
		// XINFO STREAM on the empty key replies `no such key`, and
		// handler.go:139 returns that error *without* passing it through
		// mapPublishErr — so connect-go classifies it CodeUnknown → 500.
		expected: [500],
	},
	{
		name: "HealthCheck",
		procedure: Procedure.healthCheck,
		expected: [200],
	},
] as const;

test.describe("Connect-RPC procedure registration", () => {
	for (const { name, procedure, expected } of MOUNTED) {
		test(`${name} is mounted`, { tag: "@smoke" }, async ({ api }) => {
			await expectProcedureMounted(api, procedure, expected);
		});
	}
});

test.describe("Connect-RPC protocol contract", () => {
	test("an unknown procedure 404s from the Go mux, not the Connect layer", { tag: "@contract" }, async ({
		api,
	}) => {
		// mqhub.connect.go's switch hands anything it does not recognise to
		// `http.NotFound`, so the body is Go's plain-text `404 page not found`
		// and never reaches the Connect codec. That matters to callers: a
		// connect-es client surfaces this as a transport error rather than a
		// ConnectError with a numeric code, so a `switch (error.code)` handler
		// will not see `unimplemented` here. Asserting the plain text keeps the
		// fact visible instead of implying an envelope that does not exist.
		const response = await callUnary(api, `${MQHUB_SERVICE}/NoSuchProcedure`);
		await expectStatus(response, 404);
		expect(await response.text()).toContain("404 page not found");
	});

	test("an unknown service 404s from the ServeMux", { tag: "@contract" }, async ({ api }) => {
		// One level up: main.go mounts the generated handler on the single
		// prefix `/services.mqhub.v1.MQHubService/`, so a different package or
		// version never reaches Connect at all. This is the fence against a
		// v2 proto landing without its mux entry.
		await expectStatus(await callUnary(api, "services.mqhub.v2.MQHubService/Publish"), 404);
	});

	test("a unary procedure refuses GET", { tag: "@contract" }, async ({ bare }) => {
		// None of the six RPCs is marked `idempotency_level = NO_SIDE_EFFECTS`
		// in the proto, so connect-go advertises POST only and answers 405 with
		// an `Allow` header. Worth pinning because the alternative — a GET that
		// reached `Publish` — would make every stream writable from a browser
		// address bar and from any link preview crawler.
		//
		// `expectMethodNotAllowed` is the shared spelling of this: it refuses a
		// 404 (route gone, a different failure) and requires the Allow header
		// RFC 9110 §15.5.6 makes mandatory, rather than defaulting it to "".
		expectMethodNotAllowed(await bare.get(procedurePath(Procedure.publish)), ["POST"]);
	});

	test("a malformed JSON body is invalid_argument, not a 500", { tag: "@contract" }, async ({ api }) => {
		// The codec must reject before the handler runs. A 500 here would mean
		// a panic recovered somewhere, and would be indistinguishable from
		// Redis being down for a caller reading only the status class.
		const response = await api.post(procedurePath(Procedure.publish), {
			headers: { "Content-Type": "application/json" },
			data: "{ this is not json",
		});
		await expectStatus(response, 400);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe("invalid_argument");
	});

	test("an unsupported Content-Type is rejected", { tag: "@contract" }, async ({ bare }) => {
		// connect-go negotiates the codec from Content-Type and answers 415 for
		// one it has no codec for. Pinned because the failure it prevents is
		// silent: a caller that forgot the header entirely, or sent
		// `text/plain`, must not have its body guessed at.
		const response = await bare.post(procedurePath(Procedure.healthCheck), {
			headers: { "Content-Type": "text/plain" },
			data: "{}",
		});
		await expectStatus(response, 415);
	});

	test("the Connect-Protocol-Version header is not required", { tag: "@contract" }, async ({ bare }) => {
		// main.go:96-98 constructs the handler with `WithInterceptors` only —
		// never `WithRequireConnectProtocolHeader` — so a plain
		// `POST + application/json` works. Every Hurl entry sent the header, so
		// the suite would not have noticed the option being added, which would
		// break exactly the hand-rolled curl/wget callers the header-less form
		// exists for.
		const response = await bare.post(procedurePath(Procedure.healthCheck), {
			headers: { "Content-Type": "application/json" },
			data: {},
		});
		await expectStatus(response, 200);
	});
});
