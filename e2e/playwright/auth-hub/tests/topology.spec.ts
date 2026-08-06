import { test, expect } from "../src/fixtures.js";
import { expectJson, expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { echoErrorSchema } from "../src/schemas.js";

/**
 * The route table and the listener boundary — new coverage.
 *
 * auth-hub's whole attack surface is five routes on one port
 * (`cmd/auth-hub/main.go:176-187`):
 *
 *     GET  /validate               nginx auth_request
 *     GET  /session                the SPA
 *     POST /csrf                   the SPA
 *     GET  /health                 operators
 *     GET  /internal/system-user   other services, shared-secret only
 *
 * plus an mTLS HTTPS listener that only exists when `MTLS_LISTEN=true`. This
 * file pins both halves: that nothing else answers, and that the flagged
 * listener really is unbound when the flag is off.
 *
 * Every negative asserts **404**, never 401 or 403. A 401 would mean the route
 * is registered and only a middleware stands between the caller and it; 404 is
 * the only status that says "this surface is not here". That distinction is
 * the same one `_shared/connect.ts`'s `expectProcedureMounted` makes for the
 * Connect services elsewhere in the fleet — auth-hub mounts no Connect mux at
 * all, so this is where the equivalent claim lives.
 */

/**
 * Kratos surfaces that must not be reachable through auth-hub.
 *
 * auth-hub is an identity-aware *proxy* only in the nginx `auth_request` sense
 * — it calls Kratos as a client and forwards nothing. A future "just pass
 * unmatched paths upstream" convenience would turn it into an unauthenticated
 * gateway to Kratos's AdminAPI, which can create identities and mint
 * credentials for anybody. That is the single worst thing this service could
 * accidentally become, and only reachability testing can see it.
 */
const KRATOS_PATHS_NOT_PROXIED = [
	"/admin/identities",
	"/admin/health/ready",
	"/sessions/whoami",
	"/self-service/login/browser",
] as const;

test.describe("route table", () => {
	test("an unregistered path answers 404 @contract", async ({ hub }) => {
		const response = await hub.get("/definitely-not-a-route");
		await expectStatus(response, 404);
		await expectJson(response, echoErrorSchema);
	});

	test("a registered path still answers — the control @smoke", async ({ hub }) => {
		// Without this, every 404 in the file would also be satisfied by a dead
		// container, and the whole file would prove nothing.
		await expectStatus(await hub.get("/health"), 200);
	});

	test("auth-hub publishes no Prometheus surface @contract", async ({ hub }) => {
		// Grounded in absence, and checked as absence: `auth-hub/` imports no
		// `promhttp` and `main.go` registers no `/metrics` route — auth-hub is
		// observed through OpenTelemetry traces (`utils/otel`) and structured
		// logs, not through a scrape endpoint.
		//
		// This is asserted rather than assumed because the fleet's other Go
		// services *do* expose `/metrics`, so "add one here too" is a natural
		// change — and on this service it would publish per-IP rate-limiter
		// cardinality and request paths on an unauthenticated port that nginx
		// forwards to. If that endpoint is ever wanted, it belongs on a separate
		// operator listener and this test is where the decision gets revisited.
		await expectStatus(await hub.get("/metrics"), 404);
	});

	for (const path of KRATOS_PATHS_NOT_PROXIED) {
		test(`auth-hub does not proxy ${path} @authz`, async ({ hub }) => {
			await expectStatus(await hub.get(path), 404);
		});
	}

	test("the Kratos AdminAPI does answer those paths directly — the control @contract", async ({
		kratosAdmin,
	}) => {
		// The other half of the pair above. `/admin/identities` returning 404 from
		// auth-hub only means something if the path is real somewhere, and this
		// pins that it is: the surface exists, it is reachable inside the staging
		// network, and auth-hub is not the thing exposing it.
		const response = await kratosAdmin.get("/admin/identities?page_size=1");
		await expectStatus(response, 200);
		expect(Array.isArray(await response.json())).toBe(true);
	});
});

test.describe("listener boundary", () => {
	test("the mTLS listener is not bound when MTLS_LISTEN is false @authz", async ({ hub }) => {
		// `main.go:205` starts the HTTPS server only under `MTLS_LISTEN=true`, and
		// compose.staging.yaml sets it to `false`. This is the negative half of a
		// feature flag, and it is exactly the assertion CLAUDE.md rule 8 is about:
		// "disabled" must be observable, not inferred. A unit test cannot see it —
		// the branch is in `main()` — and no HTTP status can express it either,
		// because the correct answer is that no connection is established at all.
		//
		// It also fences the inverse mistake. `tlsutil.LoadServerConfig` defaults
		// `ClientAuth` to `NoClientCert`, so a listener that came up without its
		// CA wired would serve the *entire* Echo handler — including
		// `/internal/system-user` — over TLS to any client, with no certificate
		// required. `expectConnectionRefused` distinguishes that from a genuinely
		// closed port; `expectTlsHandshakeRejected` is what this becomes if the
		// staging slice ever turns the flag on.
		await expectConnectionRefused(
			hub,
			`${env.mtlsURL}/health`,
			"auth-hub must expose no mutual-TLS listener while MTLS_LISTEN=false",
		);
	});
});
