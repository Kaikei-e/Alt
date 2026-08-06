import { expectStatus, expectStatusIn } from "../../_shared/http.js";
import { ZERO_UUID } from "../src/env.js";
import { P, expect, test } from "../src/fixtures.js";
import { PROCEDURE_PROBES, assertProcedureRegistered } from "../src/procedures.js";

/**
 * Connect-RPC service registration — new coverage.
 *
 * `AcolyteServiceASGIApplication` builds its endpoint table from a generated
 * dict of twelve entries, and `main.py` mounts that one ASGI app. The Hurl
 * suite exercised seven of the twelve and never asserted registration as a
 * property in its own right, so a regeneration of `acolyte_connect.py` that
 * dropped an entry — or a `create_app()` that constructed the service against a
 * half-wired DI graph — would have surfaced as a 404 in the BFF
 * (`compose/bff.yaml:28` points ACOLYTE_CONNECT_URL here) rather than in CI.
 *
 * The discriminator this service needs is the response *body*, not the status.
 * See `src/procedures.ts` for why 401-vs-404 cannot work here and what replaces
 * it.
 */

test.describe("every AcolyteService procedure is registered", () => {
	for (const probe of PROCEDURE_PROBES) {
		const name = probe.procedure.split("/")[1] ?? probe.procedure;
		test(`${name} answers from its own handler @contract`, async ({ acolyte }) => {
			await assertProcedureRegistered(acolyte, probe);
		});
	}
});

test.describe("paths that are not procedures", () => {
	test("an unknown method on a known service does not resolve @contract", async ({ acolyte }) => {
		// The negative control that gives the registration probes above their
		// meaning. If a made-up method answered the same way a real one does,
		// none of the twelve assertions would prove anything.
		//
		// A band, and each member has a reason: **404** is what the Connect
		// protocol prescribes for a procedure the endpoint table does not know,
		// and also what Starlette's router emits if the mount prefix does not
		// match at all; **501** is what a server that routes the path but has no
		// implementation would answer. Anything else — a 200, a 401, a 500 —
		// means an unknown name reached code that tried to serve it.
		const response = await acolyte.post("/alt.acolyte.v1.AcolyteService/NoSuchProcedure", {
			data: {},
		});
		await expectStatusIn(response, [404, 501]);
	});

	test("a different service name in the same package does not resolve @contract", async ({
		acolyte,
	}) => {
		// One package, one service. `main.py` mounts a single ASGI app, so there
		// is no second Connect service on this listener and there must never be
		// one arriving by accident through a shared mount prefix.
		const response = await acolyte.post("/alt.acolyte.v1.AcolyteAdminService/GetAnything", {
			data: {},
		});
		await expectStatusIn(response, [404, 501]);
	});

	test("a Connect error carries the envelope a generated client can decode @contract", async ({
		acolyte,
	}) => {
		// The frontend and the BFF both use connect-es, which derives a numeric
		// `ConnectError.code` from this envelope. A handler that answered 404
		// with free-form text — or with FastAPI's `{"detail": …}` — would surface
		// client-side as a transport error with no code at all, and every
		// `switch (err.code)` would fall through to the default branch
		// (`docs/best_practices/typescript.md` rule 10).
		//
		// The Hurl scenarios asserted only `jsonpath "$.code"`, which passes for
		// `{"code":"not_found"}` with no message. The `message` is what a support
		// ticket quotes, so it is part of the contract too.
		const response = await acolyte.post(`/${P.getReport}`, { data: { reportId: ZERO_UUID } });
		await expectStatus(response, 404);

		const body: unknown = await response.json();
		expect(body).toMatchObject({ code: "not_found" });
		expect(
			(body as { message?: unknown }).message,
			"a Connect error must carry a human-readable message, not just a code",
		).toEqual(expect.stringContaining(ZERO_UUID));
	});
});
