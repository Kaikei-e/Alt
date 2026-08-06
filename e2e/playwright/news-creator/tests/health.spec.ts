import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { healthSchema } from "../src/schemas.js";

/**
 * `/health` — the port of `00-setup.hurl` + `01-health-schema.hurl`.
 *
 * The retry/readiness half of `00-setup.hurl` moved to
 * `setup/global-setup.ts`; what is left here is the contract, which is what
 * compose's healthcheck, Prometheus blackbox probes and pre-processor's
 * upstream check all read.
 */
test.describe("health", () => {
	test("GET /health reports healthy, names the service and lists models @smoke @contract", async ({
		api,
	}) => {
		const response = await api.get("/health");
		const body = await expectJsonStatus(response, 200, healthSchema);

		// FastAPI's default encoder, so snake_case JSON — not the proto3-JSON
		// camelCase the Go/Connect services emit. Pinning the content type keeps
		// a future `Response(..., media_type=...)` slip visible.
		expectHeaderContains(response, "Content-Type", "application/json");

		// `models` is `list_models()` forwarded verbatim. Asserting the model the
		// slice configured (LLM_MODEL / the stub's /api/tags) is what turns
		// "some list came back" into "the gateway is talking to the LLM this
		// deployment was pointed at". A gateway wired to the wrong upstream
		// answers a well-formed, entirely wrong list.
		expect(body.models.map((model) => model.name)).toContain(env.stubModel);
	});

	test("GET /health needs no credential @smoke @authz", async ({ api }) => {
		// news-creator carries no auth middleware other than
		// `PeerIdentityMiddleware`, which runs with `strict=False` (main.py), so
		// the listener answers anonymously by design. The container healthcheck
		// (`urllib.request.urlopen('http://127.0.0.1:11434/health')`) sends no
		// headers at all and would deadlock the whole slice if that changed.
		const response = await api.get("/health", { headers: {} });
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
		// Two failures this catches. If `strict` were ever flipped to True in
		// main.py without the sidecar in front, the forged call would start
		// answering 401/403 and every unauthenticated caller in staging would
		// break. If the trust checks were dropped, the forged identity would be
		// accepted — a caller reaching the plaintext port could name itself
		// whatever it liked (infra/peer_identity.py).
		const [forged, plain] = await Promise.all([
			api.get("/health", { headers: { "X-Alt-Peer-Identity": "alt-backend" } }),
			api.get("/health"),
		]);
		expect(forged.status(), "the forged identity changed the answer").toBe(plain.status());
		await expectJsonStatus(forged, 200, healthSchema);
	});
});
