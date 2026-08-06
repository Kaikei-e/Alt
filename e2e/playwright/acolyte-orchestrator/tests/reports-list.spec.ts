import { callUnary } from "../../_shared/connect.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { testToken } from "../../_shared/ids.js";
import type { APIRequestContext } from "@playwright/test";
import { P, createReport, expect, test } from "../src/fixtures.js";
import { listReportsResponseSchema } from "../src/schemas.js";
import type { ListReportsResponse } from "../src/schemas.js";

/**
 * `ListReports` — the port of `04-list-reports.hurl`, plus the pagination the
 * Hurl file never touched.
 *
 * `04` created two reports, asked for `limit: 50` and checked both ids came
 * back. That leaves the entire cursor mechanism untested: `list_reports` in
 * postgres_report_gw.py:103-131 over-fetches by one (`LIMIT limit + 1`) to
 * decide `has_more`, and derives `next_cursor` from the *last returned row's*
 * `created_at` with a strict `created_at < %s` predicate on the next page. Both
 * the off-by-one and the strict comparison are the kind of thing that works for
 * two rows and breaks at the page boundary.
 *
 * Every test here seeds its own reports under a `testToken` title, so the
 * listing being global — it has no tenant predicate — never makes one test's
 * assertions depend on another's rows.
 */

async function list(
	api: APIRequestContext,
	request: Record<string, unknown>,
): Promise<ListReportsResponse> {
	return expectJsonStatus(
		await callUnary(api, P.listReports, request),
		200,
		listReportsResponseSchema,
	);
}

test.describe("ListReports", () => {
	test("returns the reports this test created @contract", async ({ acolyte }, testInfo) => {
		const titleA = testToken(testInfo.workerIndex, "list-a");
		const titleB = testToken(testInfo.workerIndex, "list-b");
		const idA = await createReport(acolyte, { title: titleA, reportType: "weekly_briefing" });
		const idB = await createReport(acolyte, { title: titleB, reportType: "tech_review" });

		// A limit well above anything the fleet can create between these two
		// statements. The listing is ordered `created_at DESC` and these two rows
		// are the newest at this instant, so they are at the head of the first
		// page; 100 is headroom, not a guess.
		const page = await list(acolyte, { limit: 100 });
		const reports = page.reports ?? [];

		const byId = new Map(reports.map((r) => [r.reportId, r]));
		expect(byId.has(idA), `${idA} should be in the listing`).toBe(true);
		expect(byId.has(idB), `${idB} should be in the listing`).toBe(true);

		// Title *and* report_type, not just the ids: `ReportSummary` is a
		// separate projection from `Report` (connect_service.py:155-163) and a
		// field wired to the wrong column would still return the right ids.
		expect(byId.get(idA)?.title).toBe(titleA);
		expect(byId.get(idA)?.reportType).toBe("weekly_briefing");
		expect(byId.get(idB)?.title).toBe(titleB);
		expect(byId.get(idB)?.reportType).toBe("tech_review");

		// Never run, so `latestRunStatus` is "" and proto3 omits it. The positive
		// case lives in tests/run-lifecycle.spec.ts.
		expect(byId.get(idA)?.latestRunStatus).toBeUndefined();
		expect(byId.get(idA)?.currentVersion, "a report with no run is at version 0").toBeUndefined();
	});

	test("honours limit exactly and reports there is more @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage. Seeding three guarantees the database holds strictly more
		// rows than the page size asked for, which is what makes `hasMore` and
		// `nextCursor` mean something. With `04`'s `limit: 50` against a
		// two-report database, both were always absent and the over-fetch was
		// never exercised.
		const title = testToken(testInfo.workerIndex, "list-limit");
		for (const n of [1, 2, 3]) {
			await createReport(acolyte, { title: `${title}-${n}`, reportType: "weekly_briefing" });
		}

		const page = await list(acolyte, { limit: 2 });
		expect(page.reports ?? [], "limit: 2 must return exactly two rows").toHaveLength(2);

		// `has_more` is `next_cursor is not None`, and `next_cursor` is only set
		// when the `LIMIT limit + 1` over-fetch actually came back with the extra
		// row (postgres_report_gw.py:130). Asserting both together is what pins
		// the off-by-one: a server that fetched `LIMIT limit` would return two
		// rows and claim there is nothing further.
		expect(page.hasMore, "three rows exist and two were returned").toBe(true);
		expect(page.nextCursor, "hasMore without a cursor is an unpaginable page").toBeTruthy();
	});

	test("paging by cursor reaches every seeded report without repeats @contract", async ({
		acolyte,
	}, testInfo) => {
		// New coverage, and the assertion that actually exercises
		// `WHERE created_at < %s`. Two failure modes it catches:
		//   - a cursor built from the wrong row (the over-fetched one rather than
		//     the last returned one) skips a report entirely;
		//   - a `<=` instead of `<` repeats the boundary row on every page, which
		//     a client rendering an infinite list shows as a duplicate card.
		const title = testToken(testInfo.workerIndex, "list-cursor");
		const mine = new Set<string>();
		for (const n of [1, 2, 3]) {
			mine.add(
				await createReport(acolyte, {
					title: `${title}-${n}`,
					reportType: "weekly_briefing",
				}),
			);
		}

		const seen = new Set<string>();
		const found = new Set<string>();
		let cursor: string | undefined;
		const cursorsUsed: string[] = [];
		// `limit: 1`, not a page size that could swallow all three seeded rows at
		// once. With `limit: 5` the three rows are the three newest at that
		// instant, so they all land on page 1, `found.size === mine.size` on the
		// first iteration and the loop breaks *before* a cursor is ever sent —
		// `WHERE created_at < %s` and `next_cursor = reports[-1].created_at` go
		// unexercised and the cross-page duplicate check inspects a single page,
		// where a duplicate is impossible by construction. One row per page forces
		// the boundary row to be re-read on every request, which is exactly where a
		// `<=` predicate emits the duplicate this test claims to catch.
		//
		// The bound is generous rather than tight: page 1 starts at the globally
		// newest row, so the walk also has to descend past whatever sibling workers
		// created in the window between this test's seeding and its first request.
		// Running off the end must be a failure, not an infinite loop.
		const MAX_PAGES = 60;
		let pages = 0;

		while (pages < MAX_PAGES) {
			pages += 1;
			const page: ListReportsResponse = await list(
				acolyte,
				cursor === undefined ? { limit: 1 } : { limit: 1, cursor },
			);
			for (const report of page.reports ?? []) {
				expect(
					seen.has(report.reportId),
					`${report.reportId} appeared on two pages — the cursor predicate is not strict`,
				).toBe(false);
				seen.add(report.reportId);
				if (mine.has(report.reportId)) found.add(report.reportId);
			}
			if (found.size === mine.size) break;
			if (page.hasMore !== true) break;
			cursor = page.nextCursor;
			expect(cursor, "hasMore was true, so a cursor must be supplied").toBeTruthy();
			if (cursor !== undefined) cursorsUsed.push(cursor);
		}

		expect(
			[...found].sort(),
			`paged through ${pages} page(s) and ${seen.size} report(s) without finding every seeded id`,
		).toEqual([...mine].sort());

		// The paging itself is part of the claim, not a means to it. Three seeded
		// rows at one row per page cannot be collected in fewer than three
		// requests, two of which must have carried a cursor. Without these the
		// test still passes when a regression makes `nextCursor` unusable and every
		// id happens to arrive on the first page.
		expect(
			pages,
			"three seeded reports at limit: 1 cannot be reached in fewer than three pages",
		).toBeGreaterThanOrEqual(3);
		expect(
			cursorsUsed.length,
			"the cursor predicate was never exercised — no page was requested with a cursor",
		).toBeGreaterThanOrEqual(2);
	});

	test("an omitted limit falls back to a page of 20 @contract", async ({ acolyte }, testInfo) => {
		// New coverage for connect_service.py:145 (`limit if limit > 0 else 20`).
		// int32 zero is indistinguishable from "not sent" on the wire, so the
		// default is the only thing standing between an unbounded client and a
		// full table scan.
		//
		// The precondition — more rows than one default page — is seeded here
		// rather than sampled from whatever the rest of the suite happens to have
		// left behind. Sampling was both vacuous and racy: it skipped in any run
		// that reached this test early (and always under PW_GREP), and when it did
		// not skip it read the total once and then asserted against a *second*
		// call, across which sibling workers' DeleteReport tests could drop the
		// total to exactly 20 — at which point the `LIMIT limit + 1` over-fetch
		// returns 20 rows, `next_cursor` stays None, `has_more` is omitted and
		// `hasMore === true` fails red for a reason that has nothing to do with the
		// default page size. These 21 rows belong to this test and no test deletes
		// them, so both assertions are unconditional.
		//
		// Twenty-one sequential inserts against a Python handler that four workers
		// may already be driving LangGraph pipelines through. The work is real, so
		// the budget is raised rather than the seeding being trimmed to fit.
		test.slow();

		const title = testToken(testInfo.workerIndex, "default-page");
		for (let n = 0; n < 21; n += 1) {
			await createReport(acolyte, { title: `${title}-${n}`, reportType: "weekly_briefing" });
		}

		const defaulted = await list(acolyte, {});
		expect(defaulted.reports ?? [], "the default page size is 20").toHaveLength(20);
		expect(defaulted.hasMore, "more rows exist beyond the default page").toBe(true);
	});
});
