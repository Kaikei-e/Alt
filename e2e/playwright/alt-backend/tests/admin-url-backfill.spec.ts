import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../src/http.js";
import { articleUrlBackfillSchema } from "../src/schemas.js";

/**
 * Admin: `KnowledgeHomeAdminService.EmitArticleUrlBackfill` (PR3 — ADR-000869)
 * — the port of `72-admin-emit-article-url-backfill.hurl`.
 *
 * Verifies the Connect-RPC route on the operator listener is wired and returns
 * the expected response envelope. The staging slice boots with an empty
 * `articles` table (the Atlas migrator only creates schema), so `dryRun=true`
 * must report all-zero counters and must not touch the sovereign event log.
 *
 * Why dry-run only: real-emit assertions belong to the unit test
 * (usecase/knowledge_url_backfill_usecase/usecase_test.go) and to the CDC
 * contract test (driver/sovereign_client/contract/…). The E2E proves only the
 * boundary — handler registered, request envelope decodes, response envelope
 * encodes, no panic on the empty-table path.
 *
 * Auth: KnowledgeHomeAdminService is service-to-service (BFF → backend at the
 * admin-only edge) and stages mTLS at TLS termination. The alt-staging slice
 * exposes the operator Connect-RPC port (:9102) without TLS for the E2E. The
 * browser-facing :9101 no longer serves this service — tests/topology.spec.ts
 * holds that half of the contract.
 */

test.describe("EmitArticleUrlBackfill (dry run)", () => {
	test("reports all-zero counters on an empty articles table", async ({ operator }) => {
		const body = await expectJsonStatus(
			await operator.post(
				"/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill",
				{ data: { maxArticles: 0, dryRun: true } },
			),
			200,
			articleUrlBackfillSchema,
		);

		// Field shape first: every counter must be present and integral. The
		// AppendKnowledgeEventPort signature change (the port returns
		// `(eventSeq int64, err error)`) is what keeps SkippedDuplicate honest
		// end to end; if a refactor drops one of these fields the schema fires
		// and the regression is caught at the boundary.
		//
		// The zero-valued fields only survive the wire because the operator
		// listener installs `codec.EmitUnpopulatedJSONCodec` — protojson would
		// otherwise strip them, and a missing field is indistinguishable from a
		// zero one to any consumer. That codec choice is what this schema is
		// really guarding.
		expect(body.articlesScanned).toBe(0);
		expect(body.eventsAppended).toBe(0);
		expect(body.skippedBlockedScheme).toBe(0);
		expect(body.skippedDuplicate).toBe(0);
		expect(body.moreRemaining).toBe(false);
	});

	test("round-trips maxArticles without changing the outcome", async ({ operator }) => {
		const body = await expectJsonStatus(
			await operator.post(
				"/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill",
				{ data: { maxArticles: 50, dryRun: true } },
			),
			200,
			articleUrlBackfillSchema,
		);
		expect(body.articlesScanned).toBe(0);
		expect(body.moreRemaining).toBe(false);
	});

	test("is idempotent across repeated dry runs", async ({ operator }) => {
		// New coverage. A dry run that mutated anything — a cursor, a watermark,
		// the event log — would show up as a second call disagreeing with the
		// first. "Dry" is a claim about side effects, and this is the only place
		// in the suite that tests it as one.
		const call = async () =>
			expectJsonStatus(
				await operator.post(
					"/alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill",
					{ data: { maxArticles: 10, dryRun: true } },
				),
				200,
				articleUrlBackfillSchema,
			);

		const first = await call();
		const second = await call();
		expect(second).toEqual(first);
	});
});

test.describe("operator listener projection health", () => {
	test("GetProjectionHealth is mounted", async ({ operator }) => {
		// New coverage. The admin service registers seventeen procedures and the
		// Hurl suite reached exactly one of them, so a partial registration —
		// the handler constructed, one method missing from the generated
		// interface — would have gone unnoticed. A non-404 is the whole claim.
		const response = await operator.post(
			"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
			{ data: {} },
		);
		expect(response.status(), "GetProjectionHealth must be registered").not.toBe(404);
	});

	test("GetFeatureFlags is mounted", async ({ operator }) => {
		const response = await operator.post(
			"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetFeatureFlags",
			{ data: {} },
		);
		expect(response.status(), "GetFeatureFlags must be registered").not.toBe(404);
	});

	test("an unregistered admin procedure is a Connect 404", async ({ operator }) => {
		// The control for the two assertions above: if everything on this
		// listener answered non-404, they would prove nothing.
		const response = await operator.post(
			"/alt.knowledge_home.v1.KnowledgeHomeAdminService/NoSuchProcedure",
			{ data: {} },
		);
		expect(response.status()).toBe(404);
	});
});
