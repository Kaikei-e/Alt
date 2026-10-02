import { expect, test } from "../src/fixtures.js";
import { env, Procedure, SharedCorpus } from "../src/env.js";
import { fixtureToken } from "../src/auth.js";
import { expectStatus } from "../../_shared/http.js";

for (const [label, token, restStatus, code] of [
	["missing", undefined, 401, "unauthenticated"],
	["invalid", "invalid", 403, "permission_denied"],
	["foreign owner", () => fixtureToken(SharedCorpus.bobUser), 403, "permission_denied"],
	["expired", () => fixtureToken(SharedCorpus.aliceUser, -1), 403, "permission_denied"],
] as const) {
	test(`REST and Connect reject ${label} owner proof`, { tag: "@authz" }, async ({ bare, rest }) => {
		await expectStatus(await rest.get(`/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}`), 200);
		const value = typeof token === "function" ? token() : token;
		const headers = value === undefined ? {} : { Authorization: `Bearer ${value}` };
		await expectStatus(await bare.get(`${env.baseURL}/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}`, { headers: { ...headers, "X-Alt-Peer-Identity": "alt-backend" } }), restStatus);
		const response = await bare.post(`${env.connectURL}/${Procedure.searchArticles}`, { headers: { ...headers, "Content-Type": "application/json" }, data: { query: "rust", userId: SharedCorpus.aliceUser, limit: 1 } });
		await expectStatus(response, value === undefined ? 401 : 403);
		expect((await response.json()).code).toBe(code);
	});
}
