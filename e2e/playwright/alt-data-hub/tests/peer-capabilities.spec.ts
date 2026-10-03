import { test, expect } from "../src/fixtures.js";
import { env } from "../src/env.js";
import { clientCertificates } from "../../_shared/client-auth.js";
import { expectStatus } from "../../_shared/http.js";
import { dirname, join } from "node:path";

test("an allowed pre-processor TLS peer cannot invoke backend-only mutations", { tag: "@authz" }, async ({ playwright }) => {
	const pkiDir = dirname(env.allowedCertPath);
	const peer = await playwright.request.newContext({ baseURL: env.dataHubURL, clientCertificates: clientCertificates(env.dataHubURL, join(pkiDir, "pre-processor.pem"), join(pkiDir, "pre-processor-key.pem")) });
	try {
		await expectStatus(await peer.post("/services.datahub.v1.DataHubService/GetSystemUser", { data: {} }), 200);
		const denied = await peer.post("/services.datahub.v1.DataHubService/DeleteFeedLink", { data: { feedLinkId: "00000000-0000-4000-a000-000000000001" } });
		await expectStatus(denied, 403);
		expect(await denied.text()).toContain("forbidden: peer not authorized for this procedure");
	} finally { await peer.dispose(); }
});
