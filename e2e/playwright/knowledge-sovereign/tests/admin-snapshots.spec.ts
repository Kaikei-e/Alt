import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { seedSnapshot } from "../src/admin.js";
import { snapshotListSchema, snapshotMetadataSchema } from "../src/schemas.js";

/**
 * Snapshots — `POST /admin/snapshots/create`, `GET /admin/snapshots/{latest,
 * list}` on the operator port.
 *
 * Ports `14-admin-snapshot-create.hurl`, `15-admin-snapshot-latest.hurl` and
 * `19-admin-snapshot-list.hurl`.
 *
 * A snapshot is what makes the read models genuinely disposable: it records
 * the `event_seq` boundary, the schema version and the projector build that
 * produced them, alongside a SHA-256 of each exported table. Retention refuses
 * to archive anything without one. So the fields asserted here are not
 * decoration — they are the evidence a restore would be reconstructed from.
 *
 * These specs run in the `workers: 1` "admin" project (see
 * playwright.config.ts): `latest` answers "the newest valid snapshot in the
 * database", which two concurrent writers would make a coin flip.
 */

test.describe("snapshot creation", () => {
	test(
		"POST /admin/snapshots/create records the full immutable-design envelope",
		{ tag: "@contract" },
		async ({ rpc, admin, principal }) => {
			// `14-admin-snapshot-create.hurl` asserted nine JSONPaths, and this
			// keeps all nine — plus the seven fields it never looked at, via
			// the schema: `projection_version`, `snapshot_at`,
			// `snapshot_data_path` and the three row counts. Those are exactly
			// what a restore would need and exactly what nothing else observes.
			const { metadata, eventSeq } = await seedSnapshot(rpc, admin, principal);

			expect(metadata.snapshot_type).toBe("full");
			expect(metadata.status).toBe("valid");
			// BUILD_REF=staging in compose.staging.yaml. The field exists so a
			// snapshot can be traced to the projector binary that wrote it; a
			// snapshot stamped "dev" here would mean the env var stopped
			// reaching the container.
			expect(metadata.projector_build_ref).toBe("staging");

			// The boundary must cover the event this test appended. The Hurl
			// scenario asserted only `> 0`, which the very first event in the
			// database satisfies forever — it could not tell a boundary that
			// tracks the log from one frozen at 1.
			expect(Number(metadata.event_seq_boundary)).toBeGreaterThanOrEqual(Number(eventSeq));

			// The three checksums are `sha256:<64 hex>` (the schema pins the
			// length; the Hurl regex accepted any number of hex digits). All
			// three are computed over separate gzip streams, so two of them
			// being equal is only possible when both tables are empty — which
			// they may legitimately be, hence no inequality assertion here.
			expect(metadata.items_checksum).toMatch(/^sha256:[0-9a-f]{64}$/);
			expect(metadata.digest_checksum).toMatch(/^sha256:[0-9a-f]{64}$/);
			expect(metadata.recall_checksum).toMatch(/^sha256:[0-9a-f]{64}$/);

			// `schema_version` is a hardcoded constant in config.Load, not a
			// reading of the applied migrations — it currently says "00009"
			// while knowledge-sovereign/migrations/ has reached 00030. The
			// assertion is deliberately "a non-empty string" rather than the
			// literal: pinning "00009" would make this test guard a value that
			// is already wrong, and a restore tool reading it would mis-detect
			// the schema either way. Recorded here so the drift is visible.
			expect(metadata.schema_version.length).toBeGreaterThan(0);
		},
	);

	test(
		"the create response is JSON and self-consistent with what was stored",
		{ tag: "@contract" },
		async ({ rpc, admin, principal }) => {
			// New coverage. `handleCreateSnapshot` marshals the in-memory
			// `meta` struct it just built, while `list` and `latest` read the
			// row back out of Postgres — two different paths to the same
			// document, and nothing compared them.
			const { metadata } = await seedSnapshot(rpc, admin, principal);

			const listed = await expectJsonStatus(
				await admin.get("/admin/snapshots/list"),
				200,
				snapshotListSchema,
			);
			const stored = listed.snapshots.find((s) => s.snapshot_id === metadata.snapshot_id);
			expect(stored, `snapshot ${metadata.snapshot_id} is missing from the list`).toBeDefined();

			expect(stored?.items_checksum).toBe(metadata.items_checksum);
			expect(stored?.digest_checksum).toBe(metadata.digest_checksum);
			expect(stored?.recall_checksum).toBe(metadata.recall_checksum);
			expect(stored?.event_seq_boundary).toBe(metadata.event_seq_boundary);
			expect(stored?.snapshot_data_path).toBe(metadata.snapshot_data_path);

			// `created_at` is the one field that does NOT agree, and this pins
			// current behaviour rather than desired behaviour: CreateSnapshot
			// never sets `meta.CreatedAt`, so the response carries Go's zero
			// time while the stored row carries the column's `DEFAULT now()`.
			// A client that trusted the create response's `created_at` would
			// date every snapshot to the year 1. When the handler starts
			// setting it, this assertion fails — which is the intended signal.
			expect(metadata.created_at).toBe("0001-01-01T00:00:00Z");
			expect(stored?.created_at).not.toBe("0001-01-01T00:00:00Z");
		},
	);
});

test.describe("snapshot reads", () => {
	test(
		"GET /admin/snapshots/latest returns a valid snapshot no older than the one just created",
		{ tag: "@contract" },
		async ({ rpc, admin, principal }) => {
			// Ports `15-admin-snapshot-latest.hurl`, which asserted four fields
			// against whatever the newest snapshot happened to be. Seeding one
			// first makes the assertion a comparison rather than a hope: the
			// boundary must have advanced to at least mine.
			const { metadata } = await seedSnapshot(rpc, admin, principal);

			const response = await admin.get("/admin/snapshots/latest");
			const latest = await expectJsonStatus(response, 200, snapshotMetadataSchema);
			expectHeaderContains(response, "Content-Type", "application/json");

			expect(latest.status).toBe("valid");
			expect(latest.snapshot_type).toBe("full");
			expect(latest.event_seq_boundary).toBeGreaterThanOrEqual(metadata.event_seq_boundary);
			// `GetLatestValidSnapshot` filters on status; a snapshot the
			// handler marked invalid must never be handed to a restore.
			expect(latest.status).not.toBe("invalidated");
		},
	);

	test(
		"GET /admin/snapshots/list is wrapped in a snapshots object",
		{ tag: "@contract" },
		async ({ rpc, admin, principal }) => {
			// Ports `19-admin-snapshot-list.hurl`, whose comment records why
			// the wrapper matters: before ADR-000942 this handler returned a
			// bare top-level array of PascalCase-keyed objects, which
			// `altctl home snapshot list` could never unmarshal into its
			// `{"snapshots": [...]}` + snake_case decode struct. That was a
			// hard failure in the CLI, not a silent one, and this is the fence
			// against a revert.
			const { metadata } = await seedSnapshot(rpc, admin, principal);

			const body = await expectJsonStatus(
				await admin.get("/admin/snapshots/list"),
				200,
				snapshotListSchema,
			);
			// Ports `19-admin-snapshot-list.hurl`'s `$.snapshots[0].status == "valid"`
			// as a claim about *this run's* snapshot rather than about whatever
			// happens to sort first. This is the only path that reads the
			// persisted status: the create response reports an in-memory struct,
			// and /latest filters `WHERE status = 'valid'` in the driver, so a
			// row written with the wrong status is invisible everywhere else.
			const stored = body.snapshots.find((s) => s.snapshot_id === metadata.snapshot_id);
			expect(stored, "the snapshot just created is missing from the list").toBeDefined();
			expect(
				stored?.status,
				"a snapshot created in this run must be listed as valid, not invalidated",
			).toBe("valid");
			// Ordered by **event_seq_boundary** DESC, not by time — and the
			// distinction is the point. `GetLatestValidSnapshot` uses the same
			// ordering, so "latest" in this API means "covers the most of the
			// event log", which is the only ordering a restore can reason
			// about. A snapshot written later but taken at a lower boundary
			// (a reproject replaying an older range, say) must not outrank it.
			const boundaries = body.snapshots.map((s) => s.event_seq_boundary);
			expect(boundaries).toEqual([...boundaries].sort((a, b) => b - a));
		},
	);
});
