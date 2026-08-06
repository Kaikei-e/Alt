import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { storageStatsSchema } from "../src/schemas.js";

/**
 * Storage metrics — `GET /admin/storage/stats` on the operator port.
 *
 * Ports `18-admin-storage-stats.hurl`, which asserted that `tables` was a
 * non-empty collection and that five keys on its first element `exist`. Every
 * one of those five is satisfied by an empty string or a null, so the scenario
 * could not tell a working query from one whose columns had all come back
 * NULL — which is exactly what a `pg_size_pretty` call against a dropped
 * relation produces.
 *
 * This lives in the `workers: 1` "admin" project with the other operator
 * specs. It reads nothing mutable, but keeping the whole `/admin/*` surface in
 * one project keeps the reason for that project in one place.
 */

test.describe("storage stats", () => {
	test(
		"GET /admin/storage/stats reports typed, table-level metrics",
		{ tag: "@contract" },
		async ({ admin }) => {
			const response = await admin.get("/admin/storage/stats");
			const body = await expectJsonStatus(response, 200, storageStatsSchema);
			expectHeaderContains(response, "Content-Type", "application/json");

			// The wrapper is the ADR-000942 contract: `altctl home storage`
			// decodes `{"tables": [...]}` with snake_case keys, and the handler
			// normalises a nil slice to `[]` so the CLI never sees a JSON null.
			//
			// The schema is where the real strengthening happens — it demands
			// `row_count` and `total_bytes` be numbers and `is_partitioned` a
			// boolean, none of which the Hurl `exists` checks could distinguish
			// from a null. `is_partitioned` in particular is the field that
			// tells retention which tables it may even consider.
			const events = body.tables.find((t) => t.name === "knowledge_events");
			expect(
				events,
				"the query filters `relname LIKE 'knowledge_%'`, so the append-only " +
					"event log is always in the result — its absence means the filter " +
					"or the schema moved",
			).toBeDefined();
			expect(
				events?.is_partitioned,
				"knowledge_events is RANGE-partitioned on occurred_at (migration 00006); " +
					"reporting it as unpartitioned would make retention skip it",
			).toBe(true);

			// `pg_size_pretty` output — "8192 bytes", "16 kB". Asserting it is
			// non-empty (the schema) plus numeric-prefixed (here) catches the
			// NULL-column case the `exists` check could not.
			for (const table of body.tables) {
				expect(table.total_size).toMatch(/^\d/);
				expect(table.table_size).toMatch(/^\d/);
				expect(table.index_size).toMatch(/^\d/);
			}

			// Ordered by total relation size descending, which is the ordering
			// an operator triaging disk pressure reads off the top.
			const sizes = body.tables.map((t) => t.total_bytes);
			expect(sizes).toEqual([...sizes].sort((a, b) => b - a));
		},
	);
});
