import {
	KEYWORD_CONTENT,
	KEYWORD_TITLE,
	expect,
	generateTagsForArticle,
	test,
} from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { uuid } from "../../_shared/ids.js";
import { generateTagsResponseSchema, healthSchema } from "../src/schemas.js";

/**
 * The Redis Streams round trip — the port of
 * `04-generate-tags-via-mqhub.hurl`, plus the reply arms it could not reach.
 *
 * Nothing in an HTTP suite can XREAD, so the consumer is driven through
 * mq-hub's synchronous wrapper, exactly as the Hurl suite did:
 *
 *   1. `GenerateTagsForArticle` publishes a `TagGenerationRequested` event to
 *      `alt:events:tags` with `reply_to=alt:replies:tags:<correlationId>`
 *      (mq-hub/app/usecase/generate_tags_usecase.go:96-131).
 *   2. tag-generator's `redis-streams-tags-consumer` thread picks it up, runs
 *      inference inline, and XADDs a reply
 *      (tag_generator/stream_event_handler.py:124-217).
 *   3. mq-hub XREADs the reply and returns it in the HTTP response.
 *
 * What changed in the port:
 *
 *   - **No ordering.** The Hurl suite ran `--jobs 1` so scenario 03 could warm
 *     SBERT before this one ran, and used a `retry: 3` to cover the window
 *     where the consumer had not yet joined its group. Both are now facts the
 *     readiness gate establishes once, so these tests can run on any worker
 *     in any order.
 *   - **Self-seeded ids.** Every test mints its own `articleId`, so the
 *     "concurrent requests" test below is possible at all — under `--jobs 1`
 *     with a single shared `{{run_id}}`, it was not.
 *
 * On the wire these are proto3-JSON responses from connect-go, whose default
 * `protojson.MarshalOptions` **omit zero values**. `success: false`,
 * `inferenceMs: 0` and an empty `tags` list are therefore absent rather than
 * present-and-falsy — see `generateTagsResponseSchema`.
 */
test.describe("tag generation over Redis Streams", () => {
	test("a request round-trips through the consumer and comes back with tags", {
		tag: ["@contract", "@slow"],
	}, async ({ mqhub, api, articleId }) => {
		const response = await generateTagsForArticle(mqhub, {
			articleId,
			title: KEYWORD_TITLE,
			content: KEYWORD_CONTENT,
			feedId: "playwright-e2e-feed",
			// 30s: the round trip is publish → XREADGROUP block cycle (~1s) →
			// inference → XADD → mq-hub's blocking XREAD. The model is warm by
			// the time any test runs, so this is headroom for Redis latency on
			// a shared runner, not an expected duration.
			timeoutMs: 30_000,
		});

		const body = await expectJsonStatus(response, 200, generateTagsResponseSchema);

		expect(body.success, "the consumer replied with a failure").toBe(true);
		// Proves the reply was routed by correlation id back to *this* request
		// rather than to whichever reply happened to be next in the stream.
		expect(body.articleId).toBe(articleId);
		expect(body.inferenceMs ?? 0).toBeGreaterThan(0);

		// Strengthened. The Hurl scenario asserted `$.tags isCollection`, which
		// is satisfied by an empty list and by a list of anything at all. The
		// element shape is the actual contract: mq-hub's `parseReply` maps
		// `{id,name,confidence}` into `GeneratedTag`, and a reply whose tags
		// lost their ids would arrive as a list of empty structs with no error
		// anywhere. The keyword-dense body is the same text the Hurl scenario
		// used, which is why "non-empty" is defensible here.
		const tags = body.tags ?? [];
		expect(tags.length, "keyword-dense content produced no tags").toBeGreaterThan(0);
		for (const tag of tags) {
			expect(tag.name.trim()).not.toBe("");
			expect(tag.confidence ?? 0).toBeGreaterThanOrEqual(0);
			expect(tag.confidence ?? 0).toBeLessThanOrEqual(1);
		}

		// New, and cheap: the consumer must still be alive afterwards.
		// `/health` reports 503 as soon as a supervised consumer thread marks
		// itself unhealthy (auth_service.py:502-507), so this catches the
		// failure mode where a request is answered and then kills the thread
		// that answered it — leaving the next round trip to time out with no
		// explanation.
		await expectJsonStatus(await api.get("/health"), 200, healthSchema);
	});

	test("GenerateTagsForArticle is mounted and something is consuming the stream", {
		tag: ["@smoke", "@slow"],
	}, async ({ mqhub }) => {
		// The wiring gate, and it discriminates three failures that all look
		// alike from a "not 2xx" assertion:
		//
		//   404 — the Connect handler was never registered on mq-hub's mux.
		//   501 — `NewHandlerWithGenerateTags` was called with a nil usecase,
		//         so the procedure answers `unimplemented`
		//         (mq-hub/app/connect/v1/mqhub/handler.go:196-198). This is
		//         precisely CLAUDE.md rule 8's "DI forgot to wire" arm.
		//   504 — the procedure is wired but nothing replied, i.e.
		//         tag-generator's consumer is not joined to `alt:events:tags`.
		//
		// The probe costs no inference: with no `articleId` the payload fails
		// `article_id_not_empty` before the extractor is touched
		// (stream_event_handler.py:150-167), so a reply comes back immediately
		// and a *slow* answer is itself the signal that something is wrong.
		//
		// It cannot be an empty request body: `timeoutMs` must be set,
		// or mq-hub falls back to a 60s block that outlives this test.
		const response = await generateTagsForArticle(mqhub, {
			title: "wiring probe",
			content: "wiring probe",
			timeoutMs: 15_000,
		});

		expect(
			response.status(),
			`GenerateTagsForArticle answered ${response.status()}: 404 means the handler is ` +
				`not registered, 501 means it was constructed without its usecase, and 504 ` +
				`means no consumer replied.\nbody: ${(await response.text()).slice(0, 500)}`,
		).toBe(200);

		// A 200 carrying something other than a reply envelope would mean
		// mq-hub answered without ever hearing from the consumer — and mq-hub
		// has arms that produce exactly that: `parseReply` answers 200 with
		// `errorMessage: "empty reply"` or `"parse reply: …"` when the reply
		// stream yielded nothing usable (generate_tags_usecase.go:165-193).
		// Asserting merely that *some* error message is present would accept
		// both of those, so the wiring gate would stay green with no consumer
		// on the other end. The consumer's own validation arm is the only
		// thing that writes "Invalid payload"
		// (stream_event_handler.py:158-166), so that substring is the proof
		// that tag-generator answered.
		const body = await expectJsonStatus(response, 200, generateTagsResponseSchema);
		expect(body.success ?? false, "a payload with no article id must not report success").toBe(
			false,
		);
		expect(body.errorMessage, "the reply did not come from the consumer's validation arm")
			.toContain("Invalid payload");
	});

	test("an over-long article id comes back as a validation failure, not a timeout", {
		tag: ["@contract", "@slow"],
	}, async ({ mqhub }) => {
		// New — the arm the Hurl README listed as out of scope
		// ("`TagGenerationRequested` validation failure — the reply arm that
		// sets `success=false` + `error_message`").
		//
		// `TagGenerationRequestPayload.article_id` is `Field(max_length=36)`,
		// so 40 characters is rejected by Pydantic inside the consumer, which
		// then publishes a *failure reply* rather than staying silent
		// (stream_event_handler.py:151-167). That difference is the whole
		// point: a consumer that dropped invalid payloads on the floor would
		// make every malformed request cost the caller a full timeout, and
		// mq-hub's 60s default would tie up a connection each time.
		const tooLong = `${uuid()}-overflow`;
		expect(tooLong.length).toBeGreaterThan(36);

		const response = await generateTagsForArticle(mqhub, {
			articleId: tooLong,
			title: KEYWORD_TITLE,
			content: KEYWORD_CONTENT,
			timeoutMs: 20_000,
		});

		const body = await expectJsonStatus(response, 200, generateTagsResponseSchema);

		// `success: false` is omitted by protojson, so absence *is* the false.
		expect(body.success ?? false, "an invalid payload must not report success").toBe(false);
		expect(body.errorMessage, "the failure reply must say why").toContain("Invalid payload");
		// The consumer echoes the id it was given so the caller can correlate
		// the failure with the article it sent.
		expect(body.articleId).toBe(tooLong);
		// No model ran, so no inference time was spent.
		expect(body.inferenceMs ?? 0).toBe(0);
	});

	test("two concurrent requests each get their own reply", {
		tag: ["@contract", "@slow"],
	}, async ({ mqhub }) => {
		// New, and only expressible now that tests are not serialised. mq-hub
		// mints a fresh correlation id and a dedicated reply stream per call
		// (generate_tags_usecase.go:76-77); tag-generator echoes the reply to
		// `event.metadata["reply_to"]`. If either side ever read a shared
		// stream — or if the consumer replied to the wrong `reply_to` — two
		// callers would receive each other's tags, and every serial suite in
		// the world would report green.
		//
		// The article ids are UUIDs, so a crossed reply is unmistakable.
		const first = uuid();
		const second = uuid();

		const [a, b] = await Promise.all([
			generateTagsForArticle(mqhub, {
				articleId: first,
				title: KEYWORD_TITLE,
				content: KEYWORD_CONTENT,
				timeoutMs: 30_000,
			}),
			generateTagsForArticle(mqhub, {
				articleId: second,
				title: "Distributed tracing across services",
				content:
					"Tracing correlates spans emitted by every service that handled a request, " +
					"using a trace identifier propagated over the wire. Sampling decisions are " +
					"made at the edge and honoured downstream.",
				timeoutMs: 30_000,
			}),
		]);

		const bodyA = await expectJsonStatus(a, 200, generateTagsResponseSchema);
		const bodyB = await expectJsonStatus(b, 200, generateTagsResponseSchema);

		expect(bodyA.articleId, "reply A was routed to the wrong caller").toBe(first);
		expect(bodyB.articleId, "reply B was routed to the wrong caller").toBe(second);
		expect(bodyA.success).toBe(true);
		expect(bodyB.success).toBe(true);
	});
});
