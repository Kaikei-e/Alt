import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { deepHealthSchema, healthSchema } from "../src/schemas.js";

/**
 * `/health` and `/health/deep` — the port of `00-setup.hurl` +
 * `01-health-schema.hurl`.
 *
 * The retry/readiness half of `00-setup.hurl` moved to
 * `setup/global-setup.ts`; what is left here is the contract, which is what
 * compose's healthcheck, Prometheus blackbox probes and pre-processor's
 * upstream check all read.
 *
 * The two paths are one contract with two halves: `/health` answers without
 * touching Ollama so the compose probe cannot restart-loop on an upstream
 * hiccup, and `/health/deep` is where that reachability became visible
 * (docs/runbooks/health-deep-contract.md).
 */
test.describe("health", () => {
	test("GET /health reports healthy and names the service without upstream I/O @smoke @contract", async ({
		api,
	}) => {
		const response = await api.get("/health");
		await expectJsonStatus(response, 200, healthSchema);

		// FastAPI's default encoder, so snake_case JSON — not the proto3-JSON
		// camelCase the Go/Connect services emit. Pinning the content type keeps
		// a future `Response(..., media_type=...)` slip visible.
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /health/deep reports the critical ollama check passing @smoke @contract", async ({
		api,
	}) => {
		const response = await api.get("/health/deep");
		const body = await expectJsonStatus(response, 200, deepHealthSchema);

		// `ollama` is the check `create_health_router` registers as critical, and
		// its probe fails unless `list_models()` returned a non-empty list. That
		// is the claim `/health` used to carry in `models[]`: the gateway is
		// wired and the upstream this deployment points at answered. Which model
		// it answered with is asserted where a caller can see it — generate /
		// chat / rag specs compare `body.model` to env.stubModel.
		const ollama = body.checks.find((check) => check.name === "ollama");
		expect(
			ollama,
			"`/health/deep` listed no `ollama` check — create_health_router built " +
				"its DeepHealthRunner without one (handler/health_handler.py)",
		).toBeDefined();
		expect(ollama?.critical).toBe(true);
		expect(ollama?.status).toBe("pass");
		expect(body.status).toBe("pass");
	});

	test("a junk credential does not turn /health into a 401 @smoke @authz", async ({ api }) => {
		// news-creator carries no auth middleware other than
		// `PeerIdentityMiddleware`, which runs with `strict=False` (main.py:239),
		// so the listener answers anonymously by design. The container healthcheck
		// (`urllib.request.urlopen('http://127.0.0.1:11434/health')`) sends no
		// headers at all and would deadlock the whole slice if that changed.
		//
		// Sending credentials the service must ignore rather than sending none is
		// what makes this a claim: an *absent* header proves nothing here, because
		// no test in this suite ever sends one — omitting it is the same request
		// the @smoke test above already makes. A bearer token and a Kratos session
		// cookie are the two shapes an accidentally-mounted auth middleware would
		// try to validate, and a middleware that rejected them would answer
		// 401/403 rather than the healthy envelope.
		const response = await api.get("/health", {
			headers: {
				Authorization: "Bearer not-a-real-token",
				Cookie: "ory_kratos_session=not-a-real-session",
			},
		});
		await expectJsonStatus(response, 200, healthSchema);
	});

	test("a forged X-Alt-Peer-Identity is not a credential on the plaintext port @authz", async ({
		api,
	}) => {
		// `PeerIdentityMiddleware.dispatch` honours the header only when
		// `PEER_IDENTITY_TRUSTED == "on"` AND the transport peer is loopback (the
		// pki-agent sidecar). Staging sets `PEER_IDENTITY_TRUSTED=off`, so the
		// header must be inert: a caller supplying one gets byte-for-byte the
		// same answer as a caller supplying none.
		//
		// One failure this catches, and only one. If `strict` were ever flipped to
		// True in main.py without the sidecar in front, the forged call would
		// start answering 401/403 (infra/peer_identity.py:79-87) and every
		// unauthenticated caller in staging would break.
		//
		// The other half — "the trust checks were dropped, so the forgery was
		// *accepted*" — is deliberately not asserted, because it is not
		// observable from outside. `dispatch` writes the peer onto
		// `request.state.peer_identity` and onto the *request* headers, and
		// `/health` builds its `{status, service}` dict without reading
		// either (handler/health_handler.py), so an accepted forgery is
		// byte-identical to a rejected one. Comparing this response's status to
		// an unforged one's is likewise not a claim: both traverse the same
		// branch, so they agree even when both have regressed together. Gating
		// acceptance needs a surface that echoes the peer.
		const forged = await api.get("/health", {
			headers: { "X-Alt-Peer-Identity": "alt-backend" },
		});
		await expectJsonStatus(forged, 200, healthSchema);
	});
});
