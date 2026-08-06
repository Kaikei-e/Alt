import { test, expect } from "../src/fixtures.js";
import { expectUnaryError } from "../../_shared/connect.js";
import { nowRFC3339, uuid } from "../../_shared/ids.js";
import { procedure } from "../src/env.js";

/**
 * Connect-RPC service registration — almost entirely new coverage.
 *
 * `KnowledgeSovereignService` declares **46** unary/streaming procedures
 * (`proto/services/sovereign/v1/sovereign.proto`). The Hurl suite exercised
 * twelve of them. The other thirty-four had no assertion of any kind, which
 * means a handler method deleted during a refactor, or a `SovereignHandler`
 * that stopped satisfying the generated interface and fell through to
 * `UnimplementedKnowledgeSovereignServiceHandler`, would have gone unnoticed
 * until a consumer broke in production.
 *
 * That embedded `Unimplemented...Handler` is the specific hazard here.
 * `handler.SovereignHandler` embeds it (sovereign_handler.go), which is what
 * lets the type compile while a method is missing — the generated stub answers
 * `unimplemented` (HTTP 501) instead of failing the build. So for this service
 * "the procedure exists" has three distinguishable answers and only one of
 * them is healthy:
 *
 *   404 — never registered; the whole service is not on this mux.
 *   501 — registered, but `SovereignHandler` does not implement the method
 *         and the embedded stub is answering.
 *   anything else — the real handler ran.
 *
 * This is CLAUDE.md rule 8 ("no silent fallback for unwired dependencies")
 * pushed out to the E2E boundary: the embedded stub is a silent fallback with
 * a compiler's blessing, and only a call across the wire tells it apart from
 * a working handler.
 *
 * There is no authentication on this listener, so — unlike alt-backend, where
 * 401-vs-404 is the discriminator — the discriminator here is the *business*
 * answer. Which makes the negatives below stronger than a mounting probe: for
 * 28 of the 46 an empty request has a single correct answer.
 */

/**
 * Procedures whose handler validates a required field before touching the
 * database, so an empty request has exactly one correct answer:
 * `invalid_argument` under HTTP 400.
 *
 * Asserting the code — not merely "some 4xx" — is what makes this a test of
 * `parseUUIDField` rather than of the mux. That helper exists precisely so a
 * malformed or empty `user_id`/`tenant_id` can never be silently coerced to
 * `uuid.Nil`: a Nil tenant written into `knowledge_events`, or used as a query
 * predicate, corrupts data or scopes a read to the wrong principal. A handler
 * that lost the guard would answer 200 with someone else's empty result set,
 * and every one of these tests would catch it.
 */
const INVALID_ARGUMENT_ON_EMPTY: readonly string[] = [
	// Mutation dispatchers — an unknown (here, empty) mutation_type falls to
	// the `default:` branch (handler/sovereign_handler.go).
	"ApplyProjectionMutation",
	"ApplyRecallMutation",
	"ApplyCurationMutation",
	// Projection reads — parseUUIDField("user_id", "").
	"GetKnowledgeHomeItems",
	"GetTodayDigest",
	"GetRecallCandidates",
	"CountNeedToKnowItems",
	// Events — explicit "tenant_id is required" / nil-message guards
	// (handler/rpc_infra.go).
	"GetLatestEventSeq",
	"AppendKnowledgeEvent",
	"AreArticlesVisibleInLens",
	// Projection infra / reproject / backfill — nil sub-message or UUID guards.
	"CreateProjectionVersion",
	"GetReprojectRun",
	"CreateProjectionAudit",
	"GetBackfillJob",
	// Lens (handler/rpc_lens.go).
	"ListLenses",
	"GetLens",
	"GetCurrentLensSelection",
	"ResolveLensFilter",
	"CreateLens",
	"CreateLensVersion",
	"SelectCurrentLens",
	"ClearCurrentLens",
	"ArchiveLens",
	// Recall signals and user events.
	"ListRecallSignals",
	"AppendRecallSignal",
	"AppendKnowledgeUserEvent",
	// Trail spine (handler/rpc_trail.go).
	"GetTrailFootprints",
	"GetTrailBranchesForAnchor",
];

/**
 * The remaining procedures, whose answer to an empty request is legitimately
 * open — a list handler returns 200, `ActivateProjectionVersion(0)` reports
 * "version 0 not found" as `internal`. For these the claim is narrower but
 * still the one that matters: the path resolves and the real handler, not the
 * embedded Unimplemented stub, is behind it.
 *
 * Two carry a hand-written `request` rather than `{}`. `CreateReprojectRun`
 * and `CreateBackfillJob` reach `protoToReprojectRun(nil)` /
 * `protoToBackfillJob(nil)`, which return a **zero-valued struct and a nil
 * error** — so an empty probe would insert a row keyed on the all-zero UUID
 * with Go's zero `time.Time` in its timestamp columns. That row then flows
 * back through protojson in tests/projection-infra.spec.ts, where a
 * `0001-01-01T00:00:00Z` timestamp sits exactly on the boundary
 * `timestamppb.CheckValid` accepts. Probing with a real request keeps this
 * file from seeding a trap for another one.
 */
const MOUNTED_ONLY: readonly { readonly method: string; readonly request?: unknown }[] = [
	{ method: "ListDistinctUserIDs" },
	{ method: "ListKnowledgeEvents" },
	{ method: "GetActiveProjectionVersion" },
	{ method: "ListProjectionVersions" },
	{ method: "ActivateProjectionVersion" },
	{ method: "GetProjectionCheckpoint" },
	{ method: "UpdateProjectionCheckpoint" },
	{ method: "GetProjectionFreshness" },
	{ method: "GetProjectionLag" },
	{ method: "ListReprojectRuns" },
	{
		method: "CreateReprojectRun",
		request: {
			run: {
				reprojectRunId: uuid(),
				projectionName: "pw-connect-surface-probe",
				fromVersion: "1",
				toVersion: "1",
				mode: "shadow",
				status: "probe",
				createdAt: nowRFC3339(),
			},
		},
	},
	{ method: "UpdateReprojectRun" },
	{ method: "CompareProjections" },
	{ method: "ListProjectionAudits" },
	{ method: "ListBackfillJobs" },
	{
		method: "CreateBackfillJob",
		request: {
			job: {
				jobId: uuid(),
				status: "probe",
				projectionVersion: 1,
				createdAt: nowRFC3339(),
				updatedAt: nowRFC3339(),
			},
		},
	},
	{ method: "UpdateBackfillJob" },
	// Server-streaming. A unary `Content-Type: application/json` is the wrong
	// protocol for it, so connect-go answers 415/400 — which is still proof
	// the procedure resolved. Only 404 (unregistered) or 501 (the embedded
	// stub) would be findings, and those are exactly what is asserted.
	{ method: "WatchProjectorEvents" },
];

test.describe("every declared procedure is registered and implemented", () => {
	for (const method of INVALID_ARGUMENT_ON_EMPTY) {
		test(
			`${method} rejects an empty request with invalid_argument`,
			{ tag: "@contract" },
			async ({ rpc }) => {
				await expectUnaryError(rpc, procedure(method), {}, "invalid_argument");
			},
		);
	}

	for (const { method, request } of MOUNTED_ONLY) {
		test(`${method} is mounted`, { tag: "@contract" }, async ({ rpc }) => {
			const response = await rpc.post(procedure(method), { data: request ?? {} });
			const body = (await response.text()).slice(0, 500);

			expect(
				response.status(),
				`${method} answered 404. The path resolved to nothing, so the Connect ` +
					`service is not registered on this mux at all — check ` +
					`NewKnowledgeSovereignServiceHandler in main.go, not the request.\n` +
					`body: ${body}`,
			).not.toBe(404);

			expect(
				response.status(),
				`${method} answered 501 (unimplemented). SovereignHandler embeds ` +
					`UnimplementedKnowledgeSovereignServiceHandler, so a method it does ` +
					`not implement still compiles and answers from the generated stub. ` +
					`That is the silent fallback CLAUDE.md rule 8 forbids.\n` +
					`body: ${body}`,
			).not.toBe(501);
		});
	}
});

test.describe("procedure count", () => {
	test("the probed set covers every declared procedure", { tag: "@contract" }, async () => {
		// A guard on the two lists above rather than on the service. A
		// procedure added to sovereign.proto and forgotten here would silently
		// shrink this file's coverage, and nothing else in the suite would
		// notice — the whole point of the file is that unprobed procedures are
		// invisible.
		//
		// 46 is the count in proto/services/sovereign/v1/sovereign.proto. When
		// it changes, add the new procedure to the list its empty-request
		// behaviour belongs in and bump this number in the same commit.
		expect(
			new Set([...INVALID_ARGUMENT_ON_EMPTY, ...MOUNTED_ONLY.map((p) => p.method)]).size,
		).toBe(46);
	});
});
