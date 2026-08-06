import { test, expect, callAs, procedurePath } from "../src/fixtures.js";
import { expectJson, expectStatus } from "../../_shared/http.js";
import { connectErrorSchema, ConnectCode } from "../../_shared/connect.js";
import { ABSENT, AUGUR_UNARY, STREAMING } from "../src/procedures.js";
import { users } from "../src/seed.js";

/**
 * Connect-RPC mux registration — new coverage.
 *
 * The Hurl suite probed exactly two procedures (`ListConversations`,
 * `GetConversation`) out of the six `SetupConnectHandlers` mounts, and never
 * touched `MorningLetterService` at all. A `mux.Handle` line dropped in a
 * refactor — or a handler whose DI dependency came back nil and skipped
 * registration — would have gone unnoticed until the SPA broke.
 *
 * This is CLAUDE.md rule 8's failure mode ("DI forgot to wire" is
 * indistinguishable from "intentionally disabled") pushed out to the E2E
 * boundary, where the only thing that can tell the two apart is whether the
 * path resolves at all. The discriminator is **404 vs anything else**:
 *
 *   - 401 `unauthenticated` — `extractUserID` ran, so the unary procedure is
 *     mounted behind its handler.
 *   - 415 — connect-go's `Handler.ServeHTTP` found no protocol handler for
 *     `application/json` on a *streaming* procedure. The path still resolved.
 *   - 404 — the path resolved to nothing. That is the wiring failure.
 *
 * `_shared/connect.ts`'s `expectProcedureMounted` is not used: it issues the
 * call through `callUnary`, which cannot attach the per-call `X-Alt-User-Id`
 * this service authenticates with, and its "expected" band would have to hold
 * both 401 and 415 without saying which procedure gets which.
 */

test.describe("AugurService unary procedures are mounted", () => {
	for (const [name, procedure] of Object.entries(AUGUR_UNARY)) {
		test(`${name} answers 401 to an anonymous caller`, { tag: "@authz" }, async ({
			connect,
		}) => {
			const response = await connect.post(procedurePath(procedure), { data: {} });
			expect(
				response.status(),
				`${procedure} answered ${response.status()}. 404 means the handler was ` +
					`never registered — check SetupConnectHandlers (connect/server.go:59) ` +
					`and the DI container, not the request. 401 is the expected ` +
					`"mounted, but you sent no X-Alt-User-Id".`,
			).toBe(401);
		});

		test(`${name} reaches its own handler for an authenticated caller`, {
			tag: "@contract",
		}, async ({ connect }) => {
			// Whatever an empty request means to each procedure — 200 for
			// ListConversations, invalid_argument for the two that parse an id,
			// invalid_argument for RetrieveContext's missing query — it must not
			// be 404 or 401. Those two are wiring failures, not business
			// outcomes, and this is what keeps the anonymous assertion above from
			// being satisfiable by a mux that 401s everything including nonsense.
			const response = await callAs(connect, procedure, users.empty, {});
			expect([404, 401], `${procedure} answered ${response.status()}`).not.toContain(
				response.status(),
			);
		});
	}
});

test.describe("streaming procedures are mounted", () => {
	for (const [name, procedure] of Object.entries(STREAMING)) {
		test(`${name} is registered and refuses the unary content type`, {
			tag: "@contract",
		}, async ({ connect }) => {
			// connect-go registers `application/json` as a *unary* content type
			// only. On a server-streaming procedure no protocol handler claims
			// it, so `Handler.ServeHTTP` answers 415 and advertises what it does
			// accept in `Accept-Post`. That is a mounted answer — the request
			// reached the generated handler — and it is the only cheap probe
			// available for a streaming RPC without speaking the enveloped
			// connect+json framing.
			//
			// This is the only assertion in the suite that covers
			// MorningLetterService at all: its single procedure is streaming, its
			// handler fans out to alt-backend for recent articles and to the LLM
			// for the answer, and neither exists in this slice. Registration is
			// therefore the whole of what is testable here — and it is exactly
			// the part that a DI regression would break.
			const response = await connect.post(procedurePath(procedure), { data: {} });
			await expectStatus(response, 415);
			expect(
				response.headers()["accept-post"] ?? "",
				`${procedure} answered 415 without advertising Accept-Post; that is not ` +
					`connect-go's unsupported-media-type path and may mean something else ` +
					`answered`,
			).toContain("application/connect+json");
		});
	}
});

test.describe("procedures that must not resolve on this mux", () => {
	for (const procedure of ABSENT) {
		test(`${procedure} → 404`, { tag: "@contract" }, async ({ connect }) => {
			// Two claims in one list. `MorningLetterReadService` is declared
			// beside `MorningLetterService` in
			// proto/alt/morning_letter/v2/morning_letter.proto and is implemented
			// by alt-backend; connect/server.go mounts only the *write* service,
			// so a "while I'm here" registration would give rag-orchestrator an
			// unowned read surface backed by no letter store.
			// `AugurService/NoSuchProcedure` is the control that proves 404 here
			// means "not registered" rather than "always 404".
			const response = await callAs(connect, procedure, users.empty, {});
			await expectStatus(response, 404);
		});
	}
});

test.describe("Connect error envelope", () => {
	test(
		"an unknown procedure 404s from the Go mux, not the Connect codec",
		{ tag: "@contract" },
		async ({ connect }) => {
			// connect-go's generated `NewAugurServiceHandler` routes only the
			// procedures it knows and hands anything else to `http.NotFound`, so
			// the body is Go's plain text, not a Connect error envelope.
			//
			// This matters to the SPA: connect-es surfaces a non-envelope 404 as
			// a transport error rather than a `ConnectError` with a numeric code,
			// so a client handler switching on `ConnectError.code` never sees
			// `unimplemented` here. Pinning the plain text keeps that fact
			// visible instead of implying an envelope that does not exist.
			const response = await connect.post("/alt.augur.v2.AugurService/NoSuchProcedure", {
				data: {},
			});
			await expectStatus(response, 404);
			expect(await response.text()).toContain("404 page not found");
		},
	);

	test(
		"an unauthenticated call answers a Connect-shaped 401",
		{ tag: "@authz" },
		async ({ connect }) => {
			// The frontend's connect-es client switches on the numeric
			// ConnectError code, which it can only derive from a well-formed
			// envelope. A bare Echo-style `{"error": "..."}` here would be a
			// silent client-side break, so the envelope shape — not just the
			// status — is part of the contract.
			const response = await connect.post(procedurePath(AUGUR_UNARY.listConversations), {
				data: {},
			});
			await expectStatus(response, 401);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.unauthenticated);
		},
	);

	test(
		"a malformed JSON body answers a 4xx, never a 500",
		{ tag: "@contract" },
		async ({ connect }) => {
			// A codec error is the caller's fault. A 5xx here would page an
			// operator for a client bug, and — worse — would be indistinguishable
			// from the handler panicking on the same input.
			const response = await connect.post(procedurePath(AUGUR_UNARY.listConversations), {
				headers: {
					// Set explicitly: Playwright picks `text/plain` for a string
					// body when the header is absent, and this test is about the
					// codec's reaction to bad JSON, not about content negotiation.
					"Content-Type": "application/json",
					"X-Alt-User-Id": users.empty,
				},
				data: "{ this is not json",
			});
			expect(response.status()).toBeGreaterThanOrEqual(400);
			expect(response.status()).toBeLessThan(500);
		},
	);
});
