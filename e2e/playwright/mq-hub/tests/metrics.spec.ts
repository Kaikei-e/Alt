import { expect, test } from "../src/fixtures.js";
import { callUnary, ConnectCode, expectUnaryError } from "../../_shared/connect.js";
import { expectPrometheusText, expectStatus } from "../../_shared/http.js";
import { MAX_BATCH_SIZE, Procedure } from "../src/env.js";
import {
	articleCreatedEvent,
	batchEvents,
	publishBatchRequest,
	publishRequest,
} from "../src/events.js";

/**
 * Prometheus surface — the port of `02-metrics.hurl`.
 *
 * The Hurl scenario opened with a comment that is the whole problem: "samples
 * are registered lazily (only after the first WithLabelValues call), so we
 * warm the publish counter with a no-op-ish publish before reading /metrics".
 * That warmup was one `Publish`, which registers `mqhub_publish_total` and
 * `mqhub_publish_duration_seconds` and nothing else — so the file could only
 * ever assert those two plus the eagerly-registered gauge, and the README's
 * claim that it also asserted `mqhub_errors_total` was not true of the
 * executable.
 *
 * A test can drive the code paths it wants to observe. These drive a success,
 * a validation failure and a batch first, so every family metrics.go declares
 * is registered by the time the endpoint is read — which is what turns "the
 * scrape returns 200" into "this service publishes what it is supposed to".
 * A `/metrics` answering 200 with an empty body is the classic silent
 * observability failure: the scrape succeeds, the dashboards stay blank, and
 * nothing alerts.
 *
 * Safe under `fullyParallel` despite the registry being global: every
 * assertion here is that a family or a labelled sample is *present*, which is
 * monotone. A sibling worker publishing at the same moment can only make it
 * more true — which is why this file needs no `workers: 1` project.
 */
test.describe("metrics", () => {
	test("GET /metrics publishes every mqhub family", { tag: "@contract" }, async ({ api, stream }) => {
		// 1. success path → mqhub_publish_total{status="success"} + duration.
		await expectStatus(
			await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
		);

		// 2. failure path → mqhub_errors_total. usecase.Publish records the
		//    error counter for anything the gateway rejects, so an event that
		//    fails `Validate()` is enough; no need to break Redis.
		await expectUnaryError(
			api,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent({ eventType: "" })),
			ConnectCode.invalidArgument,
		);

		// 3. batch path → mqhub_batch_size, which only RecordBatchPublish
		//    observes (metrics.go:69-73). The Hurl warmup never called it, so
		//    this family could have been unregistered for a whole release with
		//    no test noticing.
		await expectStatus(
			await callUnary(
				api,
				Procedure.publishBatch,
				publishBatchRequest(stream, [articleCreatedEvent(), articleCreatedEvent()]),
			),
			200,
		);

		// 4. /health calls PublishUsecase.HealthCheck, the only thing that ever
		//    moves the Redis gauge.
		await expectStatus(await api.get("/health"), 200);

		const body = await expectPrometheusText(await api.get("/metrics"), [
			"mqhub_publish_total",
			"mqhub_publish_duration_seconds",
			"mqhub_batch_size",
			"mqhub_errors_total",
			"mqhub_redis_connection_status",
		]);

		// The gauge's *value*, not just its name. `SetRedisDisconnected()`
		// leaves the family present with a 0, so asserting the name alone
		// cannot distinguish a healthy service from one whose Redis has gone
		// away — precisely the thing this metric exists to page on.
		expect(body, "mqhub_redis_connection_status should be 1 (connected)").toMatch(
			/^mqhub_redis_connection_status 1$/m,
		);
	});

	test("publish counters are labelled by stream and status", { tag: "@contract" }, async ({
		api,
		stream,
	}) => {
		// metrics.go labels PublishTotal with {stream,status}. Losing a label —
		// or swapping the two arguments of RecordPublish, which compiles fine
		// because both are strings — collapses every per-stream dashboard and
		// alert into one aggregate without any unit test failing. A stream key
		// unique to this test is what makes the sample matchable exactly.
		// Label names are emitted in alphabetical order by the Go client.
		await expectStatus(
			await callUnary(api, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
		);

		const body = await expectPrometheusText(await api.get("/metrics"));
		expect(body).toContain(`mqhub_publish_total{status="success",stream="${stream}"}`);
	});

	test("the oversize-batch guard is counted separately", { tag: ["@contract", "@slow"] }, async ({
		api,
		stream,
	}) => {
		// publish_usecase.go:104 records error_type="batch_too_large" *before*
		// touching Redis, distinctly from the "redis_error" every other failure
		// records. That distinction is what lets an operator tell "a client is
		// misbehaving" from "the broker is degraded" — two failures with
		// opposite runbooks. Nothing asserted it before.
		await expectUnaryError(
			api,
			Procedure.publishBatch,
			publishBatchRequest(stream, batchEvents(MAX_BATCH_SIZE + 1)),
			ConnectCode.invalidArgument,
		);

		const body = await expectPrometheusText(await api.get("/metrics"));
		expect(body).toContain(
			'mqhub_errors_total{error_type="batch_too_large",operation="publish_batch"}',
		);
	});

	test("GET /metrics needs no credentials", { tag: "@authz" }, async ({ bare }) => {
		// Prometheus scrapes this with no headers at all. Same claim as
		// /health: mq-hub mounts no auth, and the network is the boundary.
		await expectStatus(await bare.get("/metrics"), 200);
	});
});
