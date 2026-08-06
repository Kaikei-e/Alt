import { callUnary } from "../../_shared/connect.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { P, test } from "../src/fixtures.js";
import { healthCheckResponseSchema, restHealthSchema } from "../src/schemas.js";

/**
 * Liveness on both surfaces — the port of `00-setup.hurl` and `01-health-rpc.hurl`.
 *
 * The two are not redundant. `GET /health` is a plain Starlette `Route` in
 * `main.py`; `HealthCheck` is a procedure inside the mounted
 * `AcolyteServiceASGIApplication`. They fail independently: a broken
 * `AcolyteConnectService` construction leaves the REST route answering 200
 * while the entire RPC surface is gone, and that is exactly the state the
 * container healthcheck (`curl -fsS http://127.0.0.1:8090/health`) would
 * report as healthy.
 *
 * The readiness *gate* for both lives in setup/global-setup.ts. These are the
 * assertions — the gate waits, the tests judge.
 */

test.describe("liveness", () => {
	test("GET /health names the service, not just a status @smoke", async ({ rest }) => {
		// `00-setup.hurl` asserted both `$.status` and `$.service`, and the
		// second one matters more than it looks: several Alt services answer
		// `{"status":"ok"}` on :8090-ish ports, so a suite pointed at the wrong
		// container by a stale BASE_URL would otherwise report green. The schema
		// pins both to literals.
		const response = await rest.get("/health");
		await expectJsonStatus(response, 200, restHealthSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /health needs no credential of any kind @smoke @authz", async ({ rest }) => {
		// PeerIdentityMiddleware wraps *every* route, including this one
		// (main.py:183-190). Staging leaves PEER_IDENTITY_STRICT unset, so
		// `self._strict` is False and the 401 branch at peer_identity.py:106-107
		// must not fire. If it ever did, compose's healthcheck would fail and
		// the whole slice would never come up — this test is the early warning
		// for a strict-mode default that leaked into the wrong environment.
		await expectStatus(await rest.get("/health"), 200);
	});

	test("HealthCheck RPC round-trips proto3 JSON @smoke", async ({ acolyte }) => {
		// HealthCheckRequest has no fields, so `{}` is the whole request. A 200
		// with `{"status":"ok"}` proves three separate things at once: the
		// fully-qualified service name resolves in the endpoint table, the
		// request decoder accepts an empty message, and the response encoder
		// emits camelCase proto3 JSON rather than the binary codec.
		const response = await callUnary(acolyte, P.healthCheck, {});
		await expectJsonStatus(response, 200, healthCheckResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});
});

test.describe("peer identity", () => {
	test("an unrecognised X-Alt-Peer-Identity does not turn a staging call into a 401 @authz", async ({
		acolyte,
	}) => {
		// PEER_IDENTITY_TRUSTED=off in the staging slice
		// (compose.staging.yaml:832). peer_identity.py:100-102 then forces
		// `peer = ""` regardless of what the caller sent, and lines 119-120
		// delete the header before the handler sees it.
		//
		// The title says only what the assertion proves, and the two are now the
		// same size. The regressions this catches are a middleware that throws when
		// the header is present (MutableHeaders misuse) and a strict-mode default
		// that turns an unauthenticated staging caller into a 401/403 — both of
		// which would take the container healthcheck down with them.
		//
		// It deliberately does NOT claim the header was *stripped*. `health_check`
		// returns the constant `HealthCheckResponse(status="ok")`
		// (connect_service.py:439-442), so comparing a spoofed response body to an
		// unspoofed one compares one literal to itself: it holds under every
		// possible middleware behaviour short of a 500, including a middleware that
		// believed the value. Nothing on this listener echoes
		// `request.state.peer_identity`, so stripping is unobservable over HTTP by
		// design; that half is unit-tested at the middleware.
		const spoofed = await acolyte.post(`/${P.healthCheck}`, {
			headers: {
				"Content-Type": "application/json",
				"X-Alt-Peer-Identity": "definitely-not-a-real-peer",
			},
			data: {},
		});
		await expectJsonStatus(spoofed, 200, healthCheckResponseSchema);
	});
});
