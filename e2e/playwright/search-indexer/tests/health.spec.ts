import { test } from "../src/fixtures.js";
import { expectHeader, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused, expectTlsHandshakeRejected } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { restHealthSchema } from "../src/schemas.js";

/**
 * Liveness on both plaintext listeners — the port of `01-health.hurl`, plus
 * the half of it the Hurl suite never had.
 *
 * search-indexer serves *two* `/health` routes, from two different
 * `http.Server` instances started in two different goroutines, and they answer
 * deliberately different bodies:
 *
 *   :9300  `{"status":"ok"}`                              bootstrap/servers.go
 *   :9301  `{"status":"healthy","service":"connect-rpc"}` connect/v2/server.go
 *
 * The container healthcheck (`/search-indexer healthcheck`, main.go) only ever
 * probes the first, over 127.0.0.1 — so a Connect listener that failed to bind
 * leaves the container reporting healthy and every RPC caller getting
 * connection refused. That is exactly the CLAUDE.md rule 8 shape ("a
 * dependency that did not come up is indistinguishable from one that is
 * intentionally off"), and it is why the second assertion below exists.
 */
test.describe("liveness", () => {
	test("plaintext :9300 GET /health reports ok", { tag: "@smoke" }, async ({ bare }) => {
		const response = await bare.get(`${env.plaintextURL}/health`);
		await expectJsonStatus(response, 200, restHealthSchema);
		expectHeader(response, "Content-Type", "application/json");
	});

	test(
		"legacy Connect :9301 port is refused",
		{ tag: "@smoke" },
		async ({ bare }) => {
			await expectConnectionRefused(
				bare,
				`${env.retiredConnectURL}/health`,
				"search-indexer legacy :9301 listener has been retired; service listens on :9300 and mTLS :9443",
			);
		},
	);

	test("health is not gated behind any credential", { tag: "@authz" }, async ({ bare, rest }) => {
		await expectStatus(await bare.get(`${env.plaintextURL}/health`), 200);
		await expectStatus(
			await bare.get(`${env.plaintextURL}/health`, {
				headers: { Authorization: "Bearer not-a-real-token" },
			}),
			200,
		);
		await expectStatus(await rest.get("/health"), 200);
		await expectStatus(
			await rest.get("/health", {
				headers: { Authorization: "Bearer not-a-real-token" },
			}),
			200,
		);
		await expectTlsHandshakeRejected(
			bare,
			`${env.baseURL}/health`,
			"search-indexer mTLS listener requires client certificate on :9443 even for /health",
		);
	});
});
