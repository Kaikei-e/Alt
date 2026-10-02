import { test, expect } from "../src/fixtures.js";
import { expectJson, expectStatus } from "../../_shared/http.js";
import { expectTlsHandshakeRejected } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { echoErrorSchema } from "../src/schemas.js";

/** Frontend TLS serves sessions; private mTLS serves internal routes; plaintext is health-only. */

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
	test("the private listener rejects a client without a certificate @authz", async ({ hub }) => {
		// The active private listener requires a verified client certificate.
		await expectTlsHandshakeRejected(
			hub,
			`${env.mtlsURL}/health`,
			"auth-hub private listener requires a verified client certificate",
		);
	});
	test("plaintext exposes health only, and frontend TLS cannot expose internal identities", async ({ playwright, hub }) => {
		const plain = await playwright.request.newContext({ baseURL: env.plaintextURL });
		try {
			await expectStatus(await plain.get("/health"), 200);
			for (const path of ["/session", "/validate", "/internal/system-user"]) {
				await expectStatus(await plain.get(path, { headers: { "X-Internal-Auth": env.internalAuthSecret } }), 404);
			}
			await expectStatus(await hub.get("/internal/system-user", { headers: { "X-Internal-Auth": env.internalAuthSecret } }), 403);
		} finally { await plain.dispose(); }
	});
});
