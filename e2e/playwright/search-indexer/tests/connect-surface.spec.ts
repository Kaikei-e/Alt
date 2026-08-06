import { expect, test } from "../src/fixtures.js";
import {
	callUnary,
	ConnectCode,
	expectProcedureMounted,
	expectUnaryError,
	procedurePath,
} from "../../_shared/connect.js";
import {
	expectJson,
	expectJsonStatus,
	expectMethodNotAllowed,
	expectStatus,
} from "../../_shared/http.js";
import { Procedure, SharedCorpus } from "../src/env.js";
import {
	connectErrorSchema,
	connectRecapResponseSchema,
	connectSearchResponseSchema,
} from "../src/schemas.js";

/**
 * The Connect-RPC mux on :9301 — entirely new coverage.
 *
 * `e2e/hurl/search-indexer/README.md` listed this listener under "out of
 * scope": *"HTTP/2 h2c, not practical to drive from Hurl. Covered by existing
 * Go integration tests under search-indexer/app/."* Both halves of that were
 * true and neither is any longer. `h2c.NewHandler` passes an ordinary
 * HTTP/1.1 request straight through to the wrapped mux, which is exactly what
 * `APIRequestContext` sends; and a Go integration test constructs the handler
 * itself, so it cannot observe whether `bootstrap.Run` ever started the second
 * `http.Server` — the one failure this file exists to catch.
 *
 * `CreateConnectServer` registers exactly one service. The discriminator
 * throughout is **404 vs anything else**: search-indexer's plaintext listener
 * has no auth interceptor (authentication is a transport-layer concern on the
 * :9443 mTLS mux), so a mounted procedure answers a *business* status to an
 * empty request rather than the 401 that plays this role in alt-backend's
 * suite. A handler skipped because its DI dependency came back nil, or a
 * `mux.Handle` line lost in a refactor, simply stops existing — CLAUDE.md
 * rule 8, at the E2E boundary.
 */
test.describe("SearchService registration", () => {
	test("SearchArticles is mounted", { tag: "@smoke" }, async ({ connect }) => {
		// 400 is the *only* correct answer to an empty request: the handler's
		// first two guards reject an empty `query` and then an empty `user_id`
		// with `CodeInvalidArgument` (connect/v2/search/handler.go), which
		// Connect maps to HTTP 400. A 404 here means the service was never
		// registered; a 200 would mean both guards are gone.
		await expectProcedureMounted(connect, Procedure.searchArticles, [400]);
	});

	test("SearchRecaps is mounted", { tag: "@contract" }, async ({ connect }) => {
		// 400 and only 400. `searchRecapsUsecase` is wired in this slice —
		// compose.staging.yaml sets `RECAP_WORKER_URL=http://stub-backend`, so
		// `bootstrap.Run` builds the recap gateway and `EnsureRecapIndex` runs
		// against the Meilisearch globalSetup already proved healthy — and the
		// handler rejects a request carrying neither `query` nor `tag_name` with
		// `invalid_argument`.
		//
		// 501 was previously in the band, on the reading that `EnsureRecapIndex`
		// failing at boot (a deliberately non-fatal branch) leaves the usecase
		// nil and the handler answers `unimplemented`. That is the CLAUDE.md
		// rule 8 regression itself, not a second correct answer, and the next
		// test asserts a 200 on the same procedure — so admitting it here meant
		// this test declaring acceptable a state its neighbour declares wrong.
		//
		// A 404 remains the discrimination that matters: it means the *service*
		// is unregistered, which would take SearchArticles down with it.
		await expectProcedureMounted(connect, Procedure.searchRecaps, [400]);
	});

	test(
		"SearchRecaps is wired, not silently unimplemented",
		{ tag: "@contract" },
		async ({ connect }) => {
			// The CLAUDE.md rule 8 assertion for the recap path: "the dependency
			// was never wired" and "recap search is intentionally off" are
			// indistinguishable from `unimplemented` alone. `RECAP_WORKER_URL` is
			// set in this slice, so the wired state is the only correct one and a
			// 501 is a regression rather than a configuration choice.
			//
			// The `recaps` index exists (EnsureRecapIndex creates it) but holds no
			// documents — the recap index loop reads the nginx stub and cannot
			// decode its HTML — so an empty hit list is the right answer, which
			// is why `connectRecapResponseSchema` is the lenient one in
			// src/schemas.ts.
			//
			// The body is parsed all the same, and the schema carries
			// `notAnErrorEnvelope`. A 200 alone would be satisfied by an HTML
			// body, or by `{"code":"internal","message":"…"}` written under a
			// 200 — and that second shape is the live risk here, because
			// `driver/meilisearch_recap_driver.go` searches with
			// `Sort: ["executed_at:desc"]` while `EnsureIndex` enqueues
			// `UpdateSortableAttributes` without awaiting the task, so the sort
			// attribute can be unregistered at the moment of the first query.
			const response = await callUnary(connect, Procedure.searchRecaps, {
				query: "zzqqxxnorecapmatches",
				limit: 5,
			});
			await expectJsonStatus(response, 200, connectRecapResponseSchema);
		},
	);

	test("an unknown method on the service 404s", { tag: "@contract" }, async ({ connect }) => {
		// The control that gives the two "is mounted" assertions their meaning.
		// connect-go's generated `NewSearchServiceHandler` routes only the
		// procedures it knows and hands everything else to `http.NotFound`, so
		// an unregistered *method* on a registered *service* never reaches the
		// Connect codec — a connect-es client sees a transport error rather than
		// a `ConnectError` with a numeric code. If a mounted and an unmounted
		// path both answered 404, this whole file would prove nothing.
		await expectStatus(await callUnary(connect, Procedure.unknownMethod), 404);
	});

	test("an unknown service 404s one layer further out", { tag: "@contract" }, async ({
		connect,
	}) => {
		// `CreateConnectServer` hangs the generated handler off the single
		// prefix `/services.search.v2.SearchService/`, so a different package or
		// version never reaches Connect at all. This is the fence against a v3
		// proto landing without its `mux.Handle` line.
		await expectStatus(await callUnary(connect, Procedure.unknownService), 404);
	});
});

test.describe("SearchArticles request validation", () => {
	/**
	 * Every guard in `connect/v2/search/handler.go`, one test each.
	 *
	 * The REST path funnels its validation failures into an undifferentiated
	 * 500; the Connect path does not, and that is worth pinning precisely,
	 * because a generated client switches on the code and can only retry, fix
	 * or surface an error it can tell apart.
	 *
	 * Field names are the lowerCamelCase protojson spelling. connect-go's JSON
	 * codec accepts the `user_id` form too, but the camelCase one is what a
	 * generated client sends, so it is what the suite sends.
	 */
	for (const [label, request] of [
		["an empty query", { userId: "someone" }],
		["a missing user_id", { query: "rust" }],
		["an empty user_id", { query: "rust", userId: "" }],
		["a negative offset", { query: "rust", userId: "someone", offset: -1 }],
	] as const) {
		test(`${label} is invalid_argument`, { tag: "@contract" }, async ({ connect }) => {
			// `expectUnaryError` asserts both halves at once: the code in the
			// body *and* the HTTP status the Connect protocol pairs with it. A
			// handler returning the right body under the wrong status breaks
			// every generated client, and so does the reverse.
			await expectUnaryError(
				connect,
				Procedure.searchArticles,
				request,
				ConnectCode.invalidArgument,
			);
		});
	}

	test(
		"user_id is required even though REST makes it optional",
		{ tag: "@contract" },
		async ({ connect, rest }) => {
			// The two transports disagree on purpose and the disagreement is
			// load-bearing: `GET /v1/search?q=rust` with no `user_id` runs the
			// *unfiltered* `SearchArticlesUsecase` for internal RAG/BM25 callers,
			// while the Connect procedure hard-requires it. Anyone adding an
			// unfiltered branch to the RPC would be putting every tenant's
			// documents behind one plaintext, unauthenticated port.
			await expectUnaryError(
				connect,
				Procedure.searchArticles,
				{ query: "rust" },
				ConnectCode.invalidArgument,
			);
			await expectStatus(await rest.get("/v1/search?q=rust&limit=1"), 200);
		},
	);
});

test.describe("Connect protocol conformance", () => {
	test("an error body is a Connect envelope", { tag: "@contract" }, async ({ connect }) => {
		// A connect-es client derives its numeric `ConnectError.code` from this
		// envelope; a bare `{"error": "..."}` or a plain-text `http.Error` would
		// be a silent client-side break, since every `switch (error.code)` would
		// fall through to the default branch. The wire spelling is the *string*
		// `invalid_argument`, never the numeric enum the client exposes.
		const response = await callUnary(connect, Procedure.searchArticles, {});
		await expectStatus(response, 400);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe(ConnectCode.invalidArgument);
		expect(body.message ?? "").not.toBe("");
	});

	test("a unary procedure refuses GET", { tag: "@authz" }, async ({ connectBare }) => {
		// Neither RPC is marked `idempotency_level = NO_SIDE_EFFECTS` in
		// proto/services/search/v2/search.proto, so connect-go advertises POST
		// only. Worth pinning because a GET that reached `SearchArticles` would
		// make every tenant's index queryable from a browser address bar and
		// from any link-preview crawler that follows a pasted URL.
		// `expectMethodNotAllowed` rather than a local status-plus-header pair:
		// it also fails when the `Allow` header is missing outright, which the
		// `?? ""` form this replaced could not distinguish from a header that
		// simply did not name POST.
		const response = await connectBare.get(procedurePath(Procedure.searchArticles));
		expectMethodNotAllowed(response, ["POST"]);
	});

	test("a malformed JSON body is invalid_argument, not a 500", { tag: "@contract" }, async ({
		connect,
	}) => {
		// The codec must reject before the handler runs. A 500 here would be
		// indistinguishable, to a caller reading only the status class, from
		// Meilisearch being down — and the two need opposite responses.
		const response = await connect.post(procedurePath(Procedure.searchArticles), {
			headers: { "Content-Type": "application/json" },
			data: "{ this is not json",
		});
		await expectStatus(response, 400);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe(ConnectCode.invalidArgument);
	});

	test("an unsupported Content-Type is rejected", { tag: "@contract" }, async ({
		connectBare,
	}) => {
		// connect-go negotiates the codec from Content-Type and answers 415 for
		// one it has no codec for. Pinned because the failure it prevents is
		// silent: a caller that sent `text/plain` must not have its body guessed
		// at.
		const response = await connectBare.post(procedurePath(Procedure.searchArticles), {
			headers: { "Content-Type": "text/plain" },
			data: "{}",
		});
		await expectStatus(response, 415);
	});

	test("the Connect-Protocol-Version header is not required", { tag: "@contract" }, async ({
		connectBare,
	}) => {
		// `CreateConnectServer` builds the handler with `WithInterceptors` only,
		// never `WithRequireConnectProtocolHeader`, so a plain
		// `POST + application/json` works. Adding that option would break the
		// hand-rolled curl/wget callers the header-less form exists for, and
		// nothing else in the tree would notice.
		const response = await connectBare.post(procedurePath(Procedure.searchArticles), {
			headers: { "Content-Type": "application/json" },
			data: { query: SharedCorpus.rustQuery, userId: SharedCorpus.aliceUser },
		});
		// A real request, not an empty one: a 400 from the validation guards
		// would also prove the call was accepted, but it would prove it for a
		// request that never reached the handler. `alice` owns `doc-rust-tokio`
		// in the shared fixture corpus, so the correct answer is that document.
		//
		// Naming the document is what collects on that reasoning. A bare 200 is
		// also what a handler answering `{}` — or answering anything at all that
		// is not a `SearchArticlesResponse` — would produce, and then the
		// header-less request would have been shown to be *accepted* without
		// being shown to be *served*.
		const body = await expectJsonStatus(response, 200, connectSearchResponseSchema);
		expect(body.hits.map((hit) => hit.id)).toContain(SharedCorpus.aliceDocId);
	});
});
