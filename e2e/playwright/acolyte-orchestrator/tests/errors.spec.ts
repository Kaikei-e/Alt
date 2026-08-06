import { ConnectCode, callUnary, expectUnaryError } from "../../_shared/connect.js";
import { expectStatusIn } from "../../_shared/http.js";
import { testToken } from "../../_shared/ids.js";
import { ZERO_UUID } from "../src/env.js";
import { P, createReport, expect, test } from "../src/fixtures.js";
import { connectErrorSchema } from "../src/schemas.js";

/**
 * Argument validation and lookup failures — the port of
 * `23-get-report-not-found.hurl`, `24-delete-report-malformed.hurl` and
 * `25-rerun-section-empty-key.hurl`, with the four other guards in
 * `connect_service.py` that the Hurl suite never reached.
 *
 * Every one of these was `jsonpath "$.code" == "..."` in Hurl, which asserts
 * the code and nothing else. `expectUnaryError` asserts the code **and** the
 * HTTP status the Connect protocol pairs with it, because a handler that
 * returns the right body under the wrong status breaks every generated client
 * just as thoroughly as the reverse.
 */

test.describe("lookups that find nothing", () => {
	test("GetReport on an unknown but well-formed id is not_found @contract", async ({
		acolyte,
	}) => {
		// The cold "never existed" path, distinct from the "deleted a moment ago"
		// path that tests/reports-crud.spec.ts covers. Same contract, different
		// cause — a cache or a soft-delete flag introduced later would break one
		// and not the other.
		await expectUnaryError(
			acolyte,
			P.getReport,
			{ reportId: ZERO_UUID },
			ConnectCode.notFound,
		);
	});

	test("GetRunStatus on an unknown run is not_found @contract", async ({ acolyte }) => {
		// New coverage for connect_service.py:367-369. The SPA polls this
		// endpoint in a loop; if an unknown run answered 200 with a zero-valued
		// ReportRun, the client would poll a nonexistent run forever waiting for
		// a terminal state that can never arrive.
		await expectUnaryError(
			acolyte,
			P.getRunStatus,
			{ runId: ZERO_UUID },
			ConnectCode.notFound,
		);
	});

	test("StartReportRun on an unknown report is not_found, not failed_precondition @contract", async ({
		acolyte,
	}) => {
		// New coverage, and the distinction is deliberate in the source:
		// start_run_uc.py raises a plain `ValueError` for a missing report but a
		// `StartRunRejectedError` for a circuit-breaker or already-active
		// rejection, and connect_service.py:225-230 maps them to different codes
		// specifically so "a retry loop can tell them apart" (the comment at
		// line 226). A client that retried a NOT_FOUND would spin forever;
		// a client that gave up on a FAILED_PRECONDITION would abandon a report
		// that is merely busy.
		await expectUnaryError(
			acolyte,
			P.startReportRun,
			{ reportId: ZERO_UUID },
			ConnectCode.notFound,
		);
	});

	test("RerunSection on an unknown report is not_found @contract", async ({ acolyte }) => {
		// New coverage for rerun_section_uc.py:30-33. The section key must be
		// non-empty or the INVALID_ARGUMENT guard at connect_service.py:389
		// short-circuits first and this would test the wrong branch.
		await expectUnaryError(
			acolyte,
			P.rerunSection,
			{ reportId: ZERO_UUID, sectionKey: "overview" },
			ConnectCode.notFound,
		);
	});

	test("RerunSection on a real report with an unknown section is not_found @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage for rerun_section_uc.py:35-39 — the *second* lookup, which
		// only runs once the report itself resolved. Seeding a real report is
		// what makes this test the other branch rather than a duplicate of the
		// one above.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "rerun-unknown-section"),
			reportType: "weekly_briefing",
		});
		await expectUnaryError(
			acolyte,
			P.rerunSection,
			{ reportId, sectionKey: "no-such-section" },
			ConnectCode.notFound,
		);
	});
});

test.describe("malformed arguments", () => {
	test("DeleteReport with a non-UUID id is invalid_argument @contract", async ({ acolyte }) => {
		// connect_service.py:420-423 is the only handler that wraps `UUID(...)`
		// in its own try/except, so it is the only one that can distinguish
		// "you sent nonsense" from "there is no such row". This is the contract
		// lock for that.
		await expectUnaryError(
			acolyte,
			P.deleteReport,
			{ reportId: "not-a-uuid" },
			ConnectCode.invalidArgument,
		);
	});

	test("RerunSection with an empty section key is invalid_argument @contract", async ({
		acolyte,
	}) => {
		// connect_service.py:389-390. Ordering matters here and is asserted by
		// the choice of report id: ZERO_UUID does not exist, so if the empty-key
		// guard were ever moved below the repository lookup this would come back
		// as not_found instead and the test would fail — which is the correct
		// outcome, because the caller's real problem is the missing key.
		await expectUnaryError(
			acolyte,
			P.rerunSection,
			{ reportId: ZERO_UUID, sectionKey: "" },
			ConnectCode.invalidArgument,
		);
	});

	test("RerunSection with a non-UUID id is reported as not_found @contract", async ({
		acolyte,
	}) => {
		// New coverage, and this asserts an *inconsistency* rather than endorsing
		// it. `uc.execute(UUID(request.report_id), ...)` at connect_service.py:394
		// evaluates the `UUID(...)` conversion inside the `try:` whose only
		// handler is `except ValueError -> Code.NOT_FOUND`, so a malformed id
		// takes the same exit as a missing report — where DeleteReport, three
		// handlers down, calls the same thing invalid_argument.
		//
		// Pinning it is the point. Either the behaviour is intentional and this
		// documents it, or it gets fixed and this test is what forces the fix to
		// be a deliberate, reviewed change rather than a silent one.
		await expectUnaryError(
			acolyte,
			P.rerunSection,
			{ reportId: "not-a-uuid", sectionKey: "overview" },
			ConnectCode.notFound,
		);
	});

	test("GetReport with a non-UUID id never answers 200 or 404 @contract", async ({
		acolyte,
	}) => {
		// New coverage for the gap at connect_service.py:87: unlike
		// delete_report, `GetReport` calls `UUID(request.report_id)` outside any
		// try/except, so a malformed id escapes as an unhandled ValueError.
		//
		// A band, and each member has a reason:
		//   - **500** is today's behaviour — connect-python turns an exception
		//     that is not a ConnectError into an `internal`/`unknown` response.
		//   - **400** is what it becomes the day someone gives this handler the
		//     same guard delete_report already has, and that fix must not require
		//     editing this test.
		// What is excluded is the whole point: **200** would mean a malformed id
		// resolved to somebody's report, and **404** would mean the service
		// reports "no such report" for input it never even parsed — sending a
		// client into a create-retry loop over a typo.
		const response = await callUnary(acolyte, P.getReport, { reportId: "not-a-uuid" });
		await expectStatusIn(response, [400, 500]);

		// Whatever the code, it has to arrive in an envelope connect-es can
		// decode; an HTML 500 page or a bare traceback is a client-side crash.
		const parsed: unknown = JSON.parse(await response.text());
		expect(
			connectErrorSchema.safeParse(parsed).success,
			`GetReport must fail through the Connect codec: ${JSON.stringify(parsed).slice(0, 400)}`,
		).toBe(true);
	});
});
