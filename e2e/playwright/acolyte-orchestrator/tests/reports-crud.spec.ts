import { ConnectCode, callUnary, expectUnaryError } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { testToken } from "../../_shared/ids.js";
import { P, createReport, expect, fetchReport, test } from "../src/fixtures.js";
import {
	deleteReportResponseSchema,
	listReportVersionsResponseSchema,
} from "../src/schemas.js";

/**
 * Report CRUD — the port of `02-crud-no-scope.hurl` and `03-crud-with-scope.hurl`,
 * with the scope round-trip extended well past what those two asserted.
 *
 * Hurl captures are file-scoped, which is why each of those files had to hold a
 * whole create→read→delete chain and why the suite ran `--jobs 1`. Nothing here
 * needs that: every test mints its own title from `testToken`, so four workers
 * can drive four independent lifecycles through one acolyte-db at once and no
 * test can observe another's rows.
 */

test.describe("report lifecycle", () => {
	test("a report with no scope is created, read back and deleted @contract", async ({
		acolyte,
	}, testInfo) => {
		const title = testToken(testInfo.workerIndex, "crud-no-scope");
		const reportId = await createReport(acolyte, { title, reportType: "weekly_briefing" });

		const read = await fetchReport(acolyte, reportId);
		expect(read.report.reportId).toBe(reportId);
		expect(read.report.title).toBe(title);
		expect(read.report.reportType).toBe("weekly_briefing");

		// `currentVersion` and `sections` are *absent*, not zero and not empty.
		// The Hurl file spelled this `jsonpath "$.report.currentVersion" not
		// exists`, and it is a real contract rather than a quirk: proto3 JSON
		// omits zero-valued scalars, so a client that reads `report.currentVersion
		// ?? 0` is correct and one that requires the key is broken. If the server
		// ever switched to `including_default_value_fields`, every "absent means
		// zero" assumption in src/schemas.ts would change at once — this is where
		// that shows up.
		expect(read.report.currentVersion, "a report with no run is at version 0").toBeUndefined();
		expect(read.sections, "a report with no run has committed no sections").toBeUndefined();
		expect(read.report.scope, "no topic was supplied, so no brief was inserted").toBeUndefined();
		expect(read.activeRun, "no run was started").toBeUndefined();

		// ListReportVersions on a report that has never run: an empty page,
		// which proto3 renders as an object with no keys at all.
		const versions = await expectJsonStatus(
			await callUnary(acolyte, P.listReportVersions, { reportId, limit: 10 }),
			200,
			listReportVersionsResponseSchema,
		);
		expect(versions.versions).toBeUndefined();
		expect(versions.hasMore).toBeUndefined();
		expect(versions.nextCursor).toBeUndefined();

		// DeleteReport answers with an empty message — DeleteReportResponse has
		// no fields (acolyte.proto:184).
		await expectJsonStatus(
			await callUnary(acolyte, P.deleteReport, { reportId }),
			200,
			deleteReportResponseSchema,
		);

		await expectUnaryError(acolyte, P.getReport, { reportId }, ConnectCode.notFound);
	});

	test("deleting twice is not idempotent — the second call is not_found @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage, and a deliberate *strengthening*. `11-delete-during-active-run.hurl`
		// ended with `status toString matches "^(200|404)$"` on its cleanup
		// delete, which cannot fail: both answers were accepted because the file
		// could not know whether the earlier delete had landed. Here the first
		// delete is known to have succeeded, so the second has exactly one
		// correct answer — connect_service.py:425-427 looks the report up before
		// deleting and raises NOT_FOUND when it is gone.
		//
		// The failure mode this catches is a handler that starts swallowing the
		// miss and returning 200: a client retrying a timed-out delete would then
		// be told it deleted something it did not, which is precisely the signal
		// an audit trail needs.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "delete-twice"),
			reportType: "weekly_briefing",
		});

		await expectJsonStatus(
			await callUnary(acolyte, P.deleteReport, { reportId }),
			200,
			deleteReportResponseSchema,
		);
		await expectUnaryError(acolyte, P.deleteReport, { reportId }, ConnectCode.notFound);
	});
});

test.describe("scope brief round-trip", () => {
	test("topic and an unknown key survive the report_briefs round-trip @contract", async ({
		acolyte,
	}, testInfo) => {
		// The port of `03-crud-with-scope.hurl`. `scope` is a
		// `map<string,string>` on the wire but a *typed* row in the database:
		// connect_service.py:79-81 converts it with `ReportBrief.from_scope`,
		// which splits the map into columns, and `GetReport` reconstructs it with
		// `to_scope()` (domain/brief.py:71-82).
		//
		// `dateRange` is the interesting half. It is not in `KNOWN_SCOPE_KEYS`
		// (brief.py:11), so it lands in `constraints_jsonb` and comes back out
		// through the constraints loop at brief.py:80-81. A refactor that dropped
		// unknown keys instead of preserving them would lose every caller-defined
		// scope parameter and nothing but this assertion would notice.
		const title = testToken(testInfo.workerIndex, "scope-topic");
		const reportId = await createReport(acolyte, {
			title,
			reportType: "market_analysis",
			scope: { topic: "AI infrastructure trends", dateRange: "2026-04" },
		});

		const read = await fetchReport(acolyte, reportId);
		expect(read.report.title).toBe(title);
		expect(read.report.reportType).toBe("market_analysis");
		expect(read.report.scope).toMatchObject({
			topic: "AI infrastructure trends",
			dateRange: "2026-04",
		});
		// market_analysis is not weekly_briefing, so no default window is
		// synthesised — the contrast with the next test is the whole point.
		expect(read.report.scope?.["time_range"]).toBeUndefined();

		await expectJsonStatus(
			await callUnary(acolyte, P.deleteReport, { reportId }),
			200,
			deleteReportResponseSchema,
		);
	});

	test("weekly_briefing gains a P7D time_range the caller never sent @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage. brief.py:48-49 injects `_WEEKLY_BRIEFING_DEFAULT_WINDOW`
		// when a weekly_briefing arrives without a `time_range`, and the comment
		// there says why: without it the Gatherer mixes months-old articles into
		// a report titled "this week".
		//
		// This is a *derived* value on a read path — the caller can never see it
		// except by reading the report back — so it is exactly the kind of rule
		// that decays silently. The assertion is on the literal `P7D` because the
		// window length is the business fact, not merely "some time_range".
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "weekly-default-window"),
			reportType: "weekly_briefing",
			scope: { topic: "AI infrastructure trends" },
		});

		const read = await fetchReport(acolyte, reportId);
		expect(read.report.scope).toMatchObject({
			topic: "AI infrastructure trends",
			time_range: "P7D",
		});
	});

	test("entities and exclude are normalised through their CSV encoding @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage. `entities` and `exclude` cross the boundary as
		// comma-separated strings, become `text[]` columns, and are re-joined on
		// the way out (brief.py:39-43 and 76-79). The round-trip is lossy on
		// purpose — surrounding whitespace is stripped and empty segments are
		// dropped — so sending `" GPU , , Northfield Compute "` and getting back
		// `"GPU,Northfield Compute"` is the contract, not a bug.
		//
		// Pinning the normalised form matters because the gatherer node builds
		// its query from these: a change that started preserving the blanks would
		// put an empty entity into the search request.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "scope-csv"),
			reportType: "market_analysis",
			scope: {
				topic: "AI infrastructure trends",
				entities: " GPU , , Northfield Compute ",
				exclude: "crypto , ",
			},
		});

		const read = await fetchReport(acolyte, reportId);
		expect(read.report.scope).toMatchObject({
			topic: "AI infrastructure trends",
			entities: "GPU,Northfield Compute",
			exclude: "crypto",
		});
	});

	test("a scope with no topic inserts no brief at all @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage, and the negative half of the branch at
		// connect_service.py:79 (`if scope.get("topic")`). A scope carrying only
		// unknown keys must not create a report_briefs row, because
		// `ReportBrief.from_scope` raises on a blank topic (brief.py:35-37) and
		// the guard is the only thing standing between that and a 500.
		//
		// Observable consequence: `GetReport` finds no brief, so `scope` is
		// omitted entirely rather than echoed back with the keys that were sent.
		const reportId = await createReport(acolyte, {
			title: testToken(testInfo.workerIndex, "scope-no-topic"),
			reportType: "tech_review",
			scope: { dateRange: "2026-04" },
		});

		const read = await fetchReport(acolyte, reportId);
		expect(
			read.report.scope,
			"no topic means no report_briefs row, so nothing to echo back",
		).toBeUndefined();
	});
});
