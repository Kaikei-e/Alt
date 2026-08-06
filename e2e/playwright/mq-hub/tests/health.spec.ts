import { expect, test } from "../src/fixtures.js";
import {
	expectHeaderContains,
	expectJsonStatus,
	expectNoHeader,
	expectStatus,
} from "../../_shared/http.js";
import { callUnary } from "../../_shared/connect.js";
import { Procedure } from "../src/env.js";
import {
	int64,
	restHealthSchema,
	restHealthShapeSchema,
	rpcHealthSchema,
	rpcHealthShapeSchema,
} from "../src/schemas.js";

/**
 * Health, on both of the surfaces that expose it — the port of
 * `00-setup.hurl`, `01-health-rest.hurl` and `03-healthcheck-rpc.hurl`.
 *
 * mq-hub answers the same three facts twice, in two spellings, because two
 * different kinds of client ask. The compose healthcheck and any ops tooling
 * speak plain HTTP and get `main.go`'s hand-rolled snake_case JSON; a
 * generated Connect client gets protojson lowerCamelCase off the same
 * `PublishUsecase.HealthCheck`. Both are contracts, and they drift
 * independently — which is why both are asserted rather than one standing in
 * for the other.
 */
test.describe("health", () => {
	test("GET /health reports healthy with Redis connected", { tag: "@smoke" }, async ({ api }) => {
		const response = await api.get("/health");
		await expectJsonStatus(response, 200, restHealthSchema);

		// main.go sets this explicitly before writing the body. Never asserted
		// by the Hurl suite, and it is the one header a `wget --spider`
		// healthcheck cannot tell you about: a handler that regressed to
		// text/plain would still pass the container probe while breaking every
		// JSON client.
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /health reports a non-negative uptime", { tag: "@smoke" }, async ({ api }) => {
		const body = await expectJsonStatus(await api.get("/health"), 200, restHealthSchema);
		// Parity with 01-health-rest.hurl's `uptime_seconds >= 0`. The schema
		// already refuses a non-number; this refuses the negative value a clock
		// stepping backwards over `time.Since(startTime)` would produce.
		expect(body.uptime_seconds).toBeGreaterThanOrEqual(0);
	});

	test("HealthCheck RPC reports healthy with Redis connected", { tag: "@smoke" }, async ({ api }) => {
		const response = await callUnary(api, Procedure.healthCheck, {});
		await expectJsonStatus(response, 200, rpcHealthSchema);

		// Parity with 03-healthcheck-rpc.hurl. The Connect JSON codec is what
		// makes this assertion meaningful: a handler mounted with the protobuf
		// codec by mistake would answer 200 with an unparseable body.
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("HealthCheck reports a non-zero uptime that never goes backwards", { tag: "@contract" }, async ({
		api,
	}) => {
		// 03-healthcheck-rpc.hurl asserted `uptimeSeconds exists`, which a
		// hardcoded constant satisfies. rpcHealthSchema restores the presence
		// half (the field is required there, not optional); these two
		// assertions are the value half.
		//
		// The absolute check is the one with teeth. It fails for a handler that
		// answers a hardcoded 0, and — because protojson omits zero values, so
		// "0" and "gone" are the same wire shape — for one that stopped
		// deriving the field from `time.Since(u.startTime)`
		// (publish_usecase.go:169) at all. It is safe because mq-hub is only
		// reported healthy after a /health probe passes (compose start_period
		// 5s plus a 3s interval) and setup/global-setup.ts probes again before
		// the first test runs, so the process is several seconds old here.
		//
		// The `>=` below is deliberately weak, and the title says so rather
		// than claiming "advances": the field is whole seconds and the two
		// calls are issued back to back, so equality is the expected outcome
		// and a genuinely advancing value could only be observed by sleeping a
		// second — a cost this suite does not pay. What it still catches is a
		// clock stepping backwards over `time.Since`.
		const first = await expectJsonStatus(
			await callUnary(api, Procedure.healthCheck, {}),
			200,
			rpcHealthSchema,
		);
		expect(
			int64(first.uptimeSeconds),
			"uptimeSeconds must be derived from the process start time; 0 (or an " +
				"omitted field, which is the same thing on the wire) means it is not",
		).toBeGreaterThan(0);

		const second = await expectJsonStatus(
			await callUnary(api, Procedure.healthCheck, {}),
			200,
			rpcHealthSchema,
		);
		expect(int64(second.uptimeSeconds)).toBeGreaterThanOrEqual(int64(first.uptimeSeconds));
	});

	test("the two health surfaces agree with each other", { tag: "@contract" }, async ({ api }) => {
		// Both read the same `PublishUsecase.HealthCheck`, so a disagreement
		// means one of the two response mappings has drifted — the failure mode
		// where the RPC still says "connected" because `HealthCheckResponse` is
		// built from `health.RedisStatus` while `/health` recomputes the string
		// from `health.Healthy`. They are genuinely two code paths in main.go
		// and handler.go, and only a cross-check catches that.
		//
		// Parsed with the *shape* schemas on purpose. restHealthSchema and
		// rpcHealthSchema pin both fields to literals, so parsing with them
		// would make the two comparisons below `true === true` and
		// `"connected" === "connected"` — a test that cannot fail, and one
		// whose claim its two siblings already make independently. Un-pinning
		// the values is what leaves the cross-check load-bearing.
		const rest = await expectJsonStatus(await api.get("/health"), 200, restHealthShapeSchema);
		const rpc = await expectJsonStatus(
			await callUnary(api, Procedure.healthCheck, {}),
			200,
			rpcHealthShapeSchema,
		);
		expect(rpc.healthy, "HealthCheck and GET /health disagree on `healthy`").toBe(rest.healthy);
		expect(
			rpc.redisStatus,
			"HealthCheck's redisStatus is passed through from usecase.HealthStatus while " +
				"GET /health recomputes the string from health.Healthy — they have drifted",
		).toBe(rest.redis_status);
	});

	test("GET /health needs no credentials", { tag: "@authz" }, async ({ bare }) => {
		// Not a formality: mq-hub mounts *no* authentication of any kind, and
		// `middleware.PeerIdentityMiddleware` is documented as unwired dead code
		// (peer_identity.go's package comment; main.go logs `peer_identity_disabled`
		// at startup). If auth is ever added, /health must stay exempt or the
		// container healthcheck — `wget --spider http://127.0.0.1:9500/health`
		// with no headers — starts failing and compose never reports the
		// service healthy. tests/topology.spec.ts states the broader claim.
		await expectStatus(await bare.get("/health"), 200);
	});

	test("responses do not advertise the server stack", { tag: "@contract" }, async ({ bare }) => {
		// net/http sets no Server header and nothing in main.go adds one; this
		// is the fence against a future middleware or reverse proxy in front of
		// mq-hub that does. Cheap, and the negative direction of a header
		// assertion is the one that rots silently.
		const response = await bare.get("/health");
		expectNoHeader(response, "Server");
		expectNoHeader(response, "X-Powered-By");
	});
});
