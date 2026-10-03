import { expect, test } from "../src/fixtures.js";
import { env } from "../src/env.js";
import { clientCertificates } from "../../_shared/client-auth.js";
import { expectStatus } from "../../_shared/http.js";
import { expectTlsHandshakeRejected } from "../../_shared/net.js";

test("business requests require a verified permitted TLS peer", { tag: "@authz" }, async ({ api, playwright }) => {
	await expectStatus(await api.get("/queue/status"), 200);
	const plain = await playwright.request.newContext({ baseURL: env.plaintextURL });
	const anonymous = await playwright.request.newContext();
	const denied = await playwright.request.newContext({ baseURL: env.baseURL, clientCertificates: clientCertificates(env.baseURL, env.deniedCert, env.deniedKey) });
	try {
		for (const headers of [{}, { "X-Alt-Peer-Identity": "alt-backend" }]) {
			await expectStatus(await plain.get("/queue/status", { headers }), 401);
		}
		await expectTlsHandshakeRejected(anonymous, `${env.baseURL}/queue/status`, "news TLS requires a client certificate");
		await expectStatus(await denied.get("/queue/status"), 403);
		expect((await api.get("/queue/status")).ok()).toBe(true);
	} finally { await plain.dispose(); await anonymous.dispose(); await denied.dispose(); }
});
