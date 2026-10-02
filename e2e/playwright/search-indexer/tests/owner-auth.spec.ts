import { expect, test } from "../src/fixtures.js";
import { env, Procedure, SharedCorpus } from "../src/env.js";
import { fixtureToken } from "../src/auth.js";
import { expectStatus } from "../../_shared/http.js";
import { expectTlsHandshakeRejected } from "../../_shared/net.js";

for (const [label, token, restStatus, code] of [
	["missing", undefined, 401, "unauthenticated"],
	["invalid", "invalid", 403, "permission_denied"],
	["foreign owner", () => fixtureToken(SharedCorpus.bobUser), 403, "permission_denied"],
	["expired", () => fixtureToken(SharedCorpus.aliceUser, -1), 403, "permission_denied"],
] as const) {
	test(`REST and Connect reject ${label} owner proof`, { tag: "@authz" }, async ({ rest, connect }) => {
		await expectStatus(await rest.get(`/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}`), 200);
		const value = typeof token === "function" ? token() : token;
		const headers = { Authorization: value ? `Bearer ${value}` : "" };
		await expectStatus(await rest.get(`/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}`, { headers }), restStatus);
		const response = await connect.post(`/${Procedure.searchArticles}`, {
			headers,
			data: { query: "rust", userId: SharedCorpus.aliceUser, limit: 1 },
		});
		await expectStatus(response, restStatus);
		expect((await response.json()).code).toBe(code);
	});
}

test("REST and Connect reject uncredentialed client over TLS", { tag: "@authz" }, async ({ bare }) => {
	await expectTlsHandshakeRejected(
		bare,
		`${env.baseURL}/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}`,
		"search-indexer mTLS listener rejects uncredentialed TLS clients during handshake",
	);
	await expectTlsHandshakeRejected(
		bare,
		`${env.connectURL}/${Procedure.searchArticles}`,
		"search-indexer mTLS listener rejects uncredentialed TLS clients during handshake",
	);
});
