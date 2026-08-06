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
		// A bound, not a guess: the whole suite creates well under 100 reports,
		// and running off the end would otherwise mean an infinite loop rather
		// than a failure.
		const MAX_PAGES = 20;
		let pages = 0;

		while (pages < MAX_PAGES) {
			pages += 1;
			const page: ListReportsResponse = await list(
				acolyte,
				cursor === undefined ? { limit: 5 } : { limit: 5, cursor },
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
		}

		expect(
			[...found].sort(),
			`paged through ${pages} page(s) and ${seen.size} report(s) without finding every seeded id`,
		).toEqual([...mine].sort());
	});

	test("an omitted limit falls back to a page of 20 @contract", async ({ acolyte }) => {
		// New coverage for connect_service.py:145 (`limit if limit > 0 else 20`).
		// int32 zero is indistinguishable from "not sent" on the wire, so the
		// default is the only thing standing between an unbounded client and a
		// full table scan.
		//
		// The assertion is only meaningful once the database holds more than one
		// page, which depends on how far through the run this test lands. Rather
		// than assert a ceiling that passes vacuously, it skips and says so — a
		// vacuous green is worse than an honest skip.
		const everything = await list(acolyte, { limit: 200 });
		const total = (everything.reports ?? []).length;
		test.skip(
			total <= 20,
			`only ${total} report(s) exist, so a 20-row default page is indistinguishable ` +
				`from "everything"`,
		);

		const defaulted = await list(acolyte, {});
		expect(defaulted.reports ?? [], "the default page size is 20").toHaveLength(20);
		expect(defaulted.hasMore, "more rows exist beyond the default page").toBe(true);
	});
});
