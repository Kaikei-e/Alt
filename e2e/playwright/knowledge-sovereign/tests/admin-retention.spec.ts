import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { seedSnapshot } from "../src/admin.js";
import {
	eligiblePartitionsSchema,
	retentionRunSchema,
	retentionStatusSchema,
} from "../src/schemas.js";

/**
 * Retention — `GET /admin/retention/{eligible,status}` and
 * `POST /admin/retention/run` on the operator port.
 *
 * Ports `16-admin-retention-eligible.hurl`, `17-admin-retention-run-dry.hurl`
 * and `20-admin-retention-status.hurl`. Between them those three asserted five
 * things, three of which were `isCollection`.
 *
 * This is the most destructive surface in the service — `RunRetention` with
 * `dry_run: false` exports partitions of the append-only event log and is the
 * first half of a detach/drop pipeline — and in the staging slice it answers
 * without authentication. So the assertions that matter most here are the ones
 * about *not* acting: that an absent `dry_run` defaults to the safe path, that
 * a dry run writes no log rows, and that the safety gate on a valid snapshot
 * holds.
 *
 * These run in the `workers: 1` "admin" project; each test seeds its own
 * snapshot, so they are independent of each other and of the snapshot specs.
 */

test.describe("eligible partitions", () => {
	test(
		"GET /admin/retention/eligible returns the flat, table-tagged envelope",
		{ tag: "@contract" },
		async ({ admin }) => {
			// `16-admin-retention-eligible.hurl` asserted exactly
			// `jsonpath "$.partitions" isCollection` — true of `[]` forever,
			// and true of an array of anything at all. Its own comment explains
			// the constraint it was working around: the monthly partitions
			// migrations 00006/00007 created are hardcoded, so whether today
			// falls inside the hot window is a date-drift question, not a
			// wire-format one, and a count assertion would flake.
			//
			// That reasoning is right, and the schema respects it — the array
			// may be empty. What the schema adds is the per-row contract:
			// every partition that *is* listed must carry a table name, a
			// parseable range and numeric counts. `altctl home retention` reads
			// exactly those fields, so a renamed key would break the CLI while
			// still satisfying `isCollection`.
			const body = await expectJsonStatus(
				await admin.get("/admin/retention/eligible"),
				200,
				eligiblePartitionsSchema,
			);

			for (const partition of body.partitions) {
				// The handler only ever scans these two partitioned tables.
				expect(["knowledge_events", "knowledge_user_events"]).toContain(
					partition.table_name,
				);
				expect(
					Date.parse(partition.range_end),
					"a partition's range must be ordered",
				).toBeGreaterThan(Date.parse(partition.range_start));
			}
		},
	);
});

test.describe("dry run", () => {
	test(
		"POST /admin/retention/run with dry_run:true plans without archiving",
		{ tag: "@contract" },
		async ({ rpc, admin, principal }) => {
			// Ports `17-admin-retention-run-dry.hurl`, which asserted
			// `dry_run == true` and `error not exists`. Both are kept — the
			// error assertion is what proves the snapshot safety gate was
			// satisfied rather than silently swallowed, since `RunRetention`
			// reports its refusal in the body under a 200.
			//
			// Added: `actions`. The Hurl file never looked at it, and its wire
			// shape is a trap — `Actions []retentionAction` carries no
			// `omitempty`, so a run that planned nothing marshals it as JSON
			// **null**, not `[]`. A client doing `for (const a of res.actions)`
			// crashes on the empty case and only on the empty case.
			await seedSnapshot(rpc, admin, principal);

			const body = await expectJsonStatus(
				await admin.post("/admin/retention/run", { data: { dry_run: true } }),
				200,
				retentionRunSchema,
			);
			expect(body.dry_run).toBe(true);
			expect(body.error).toBeUndefined();
			for (const action of body.actions ?? []) {
				// A dry run marks every planned action `dry_run` and never
				// `exported`. A status of `exported` here would mean the flag
				// was ignored and partitions were actually written to disk.
				expect(action.status).toBe("dry_run");
				expect(action.action).toBe("export");
				expect(action.path).toBeUndefined();
				expect(action.checksum).toBeUndefined();
			}
		},
	);

	test(
		"POST /admin/retention/run with no body defaults to a dry run",
		{ tag: "@authz" },
		async ({ rpc, admin, principal }) => {
			// New coverage for the single most important line in
			// retention_handler.go: `DryRun *bool`. The pointer exists so an
			// absent field is distinguishable from an explicit `false`, and the
			// handler defaults the absent case to `true`.
			//
			// Change it to a plain `bool` — the obvious simplification, and one
			// no unit test of `RunRetention` would catch because that function
			// takes the flag as an argument — and an empty POST to an
			// unauthenticated endpoint starts archiving the event log for real.
			// This is the assertion that stands between those two states.
			await seedSnapshot(rpc, admin, principal);

			const body = await expectJsonStatus(
				await admin.post("/admin/retention/run"),
				200,
				retentionRunSchema,
			);
			expect(
				body.dry_run,
				"an absent dry_run must default to true; a POST with no body must never archive",
			).toBe(true);
		},
	);

	test(
		"POST /admin/retention/run rejects a malformed body",
		{ tag: "@contract" },
		async ({ admin }) => {
			// New coverage. The handler distinguishes `io.EOF` (an empty body,
			// which is legal and means "dry run") from a decode failure (which
			// is a 400). Collapsing the two — treating any decode error as an
			// empty body — would be a quiet way to reach the default path with
			// a body the caller believed said `{"dry_run": false}`.
			const response = await admin.post("/admin/retention/run", {
				headers: { "Content-Type": "application/json" },
				data: "{ not json",
			});
			await expectStatus(response, 400);
		},
	);
});

test.describe("retention log", () => {
	test(
		"GET /admin/retention/status is wrapped in a logs object",
		{ tag: "@contract" },
		async ({ admin }) => {
			// Ports `20-admin-retention-status.hurl`, whose comment records the
			// bug this wrapper fixed: the handler used to return a bare
			// top-level array — or JSON `null` when the table was empty — and
			// `altctl home retention status` could never unmarshal it into its
			// `{"logs": [...]}` decode struct. The null case decoded silently
			// into a zero-value struct, which is why the drift survived: the
			// CLI only broke once a log row existed.
			//
			// The handler now normalises nil to `[]`, so the array is present
			// even when empty; the schema requires it rather than allowing
			// `.nullable()`, which is the whole point.
			const body = await expectJsonStatus(
				await admin.get("/admin/retention/status"),
				200,
				retentionStatusSchema,
			);
			expect(Array.isArray(body.logs)).toBe(true);
		},
	);

	test(
		"a dry run appends no rows to the retention log",
		{ tag: "@contract" },
		async ({ rpc, admin, principal }) => {
			// `20-admin-retention-status.hurl` asserted `logs count == 0`,
			// which is true of a database nobody has ever run retention
			// against — an assertion about the fixture, not about the handler.
			// It also could not survive a suite where anything ran a real
			// retention.
			//
			// The claim worth making is the causal one: `logAction` is only
			// reached on the non-dry branch, so a dry run must leave the log
			// exactly as it found it. Comparing before and after says that
			// directly, and stays true however many snapshots or dry runs the
			// rest of the suite has performed.
			await seedSnapshot(rpc, admin, principal);

			const before = await expectJsonStatus(
				await admin.get("/admin/retention/status"),
				200,
				retentionStatusSchema,
			);

			const run = await expectJsonStatus(
				await admin.post("/admin/retention/run", { data: { dry_run: true } }),
				200,
				retentionRunSchema,
			);
			expect(run.dry_run).toBe(true);

			const after = await expectJsonStatus(
				await admin.get("/admin/retention/status"),
				200,
				retentionStatusSchema,
			);
			expect(
				after.logs.length,
				"a dry run must not reach logAction — it plans, it does not record",
			).toBe(before.logs.length);

			// And whatever is in the log, none of it may claim to be a dry run:
			// `logAction` is called from the export branch only.
			for (const entry of after.logs) {
				expect(entry.dry_run).toBe(false);
			}
		},
	);
});
