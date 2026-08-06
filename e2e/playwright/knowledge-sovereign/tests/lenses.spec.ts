import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { expectUnaryError } from "../../_shared/connect.js";
import { uuid } from "../../_shared/ids.js";
import { procedure } from "../src/env.js";
import { instant } from "../src/fixtures.js";
import type { Principal } from "../src/fixtures.js";
import type { APIRequestContext } from "@playwright/test";
import {
	currentLensSelectionSchema,
	getLensSchema,
	listLensesSchema,
	resolvedLensFilterSchema,
} from "../src/schemas.js";

/**
 * Lenses — the saved-view surface (`knowledge_lenses`,
 * `knowledge_lens_versions`, `knowledge_current_lens`).
 *
 * Ports `09-lens-create.hurl` through `12-lens-select-current.hurl`, which
 * were the clearest example of why the Hurl suite needed `--jobs 1`: 09 created
 * a lens, 10 created a version whose foreign key pointed at it, 12 selected
 * that pair, and 11 asserted `lenses count == 1` for a `user_id` minted once in
 * `run.sh`. Four files, one chain, no way to run any of them alone.
 *
 * Here each test creates its own lens under its own `principal.userId`, so
 * "exactly one lens" is a statement about `WHERE l.user_id = $1 AND
 * l.archived_at IS NULL` rather than about the order files happened to run in.
 * The chain that genuinely cannot be broken — a version needs its lens to
 * exist, because the FK says so — becomes a two-line seed inside the test that
 * needs it, which is what a chain should have been all along.
 *
 * Two of the Hurl README's deferred items land here: **ArchiveLens** and the
 * lens-filter resolution the SPA reads on every Home request.
 */

type SeededLens = {
	readonly lensId: string;
	readonly lensVersionId: string;
	readonly name: string;
	readonly occurredAt: string;
};

/**
 * Creates a lens and one version for `principal`.
 *
 * Throws on failure rather than returning a status: every caller uses this as
 * a precondition, and a seeding failure reported later as "ListLenses returned
 * nothing" sends the reader after the wrong handler.
 */
async function seedLens(rpc: APIRequestContext, principal: Principal): Promise<SeededLens> {
	const lensId = uuid();
	const lensVersionId = uuid();
	const name = `playwright lens ${principal.token}`;
	const { occurredAt } = instant();

	const created = await rpc.post(procedure("CreateLens"), {
		data: {
			lens: {
				lensId,
				userId: principal.userId,
				tenantId: principal.tenantId,
				name,
				description: "Playwright E2E lens",
				createdAt: occurredAt,
			},
		},
	});
	if (created.status() !== 200) {
		throw new Error(`CreateLens failed with ${created.status()}: ${await created.text()}`);
	}

	const version = await rpc.post(procedure("CreateLensVersion"), {
		data: {
			version: {
				lensVersionId,
				lensId,
				queryText: "playwright",
				tagIds: ["alpha", "beta"],
				sourceIds: [],
				timeWindow: "7d",
				includeRecap: true,
				includePulse: true,
				sortMode: "relevance",
				createdAt: occurredAt,
			},
		},
	});
	if (version.status() !== 200) {
		throw new Error(
			`CreateLensVersion failed with ${version.status()}: ${await version.text()}`,
		);
	}

	return { lensId, lensVersionId, name, occurredAt };
}

test.describe("create and list", () => {
	test(
		"CreateLens answers an empty response and the lens becomes listable",
		{ tag: "@smoke" },
		async ({ rpc, principal }) => {
			// `09-lens-create.hurl` asserted HTTP 200 and nothing else — the
			// response message has no fields, so 200 was all Hurl could see,
			// and a handler that returned early without writing would have
			// passed. `11-lens-list.hurl` was the read-back, three files later.
			// Doing both here means the write is verified by its effect.
			const seeded = await seedLens(rpc, principal);

			const body = await expectJsonStatus(
				await rpc.post(procedure("ListLenses"), { data: { userId: principal.userId } }),
				200,
				listLensesSchema,
			);
			expect(body.lenses).toHaveLength(1);
			const lens = body.lenses?.[0];
			expect(lens?.lensId).toBe(seeded.lensId);
			expect(lens?.userId).toBe(principal.userId);
			expect(lens?.tenantId).toBe(principal.tenantId);
			expect(lens?.name).toBe(seeded.name);
			// Not asserted by the Hurl suite: `ListLenses` joins the newest
			// non-superseded version with a LATERAL subquery, so a lens listed
			// without its `currentVersion` means the join broke and every
			// client would fall back to an unfiltered Home.
			expect(lens?.currentVersion?.lensVersionId).toBe(seeded.lensVersionId);
			expect(lens?.archivedAt).toBeUndefined();
		},
	);

	test(
		"GetLens returns the lens with its current version",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `10-lens-version-create.hurl` asserted HTTP 200 on
			// the write and never read the version back, so every field of it
			// — query text, tags, the time window, the booleans — was
			// unverified. Those fields *are* the lens; a version that persists
			// with its filter silently dropped produces a lens that matches
			// everything.
			const seeded = await seedLens(rpc, principal);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetLens"), { data: { lensId: seeded.lensId } }),
				200,
				getLensSchema,
			);
			expect(body.lens.lensId).toBe(seeded.lensId);
			const version = body.lens.currentVersion;
			expect(version?.lensVersionId).toBe(seeded.lensVersionId);
			expect(version?.queryText).toBe("playwright");
			expect(version?.tagIds).toEqual(["alpha", "beta"]);
			// `time_window_json` is a jsonb column holding a JSON *string*
			// (`json.Marshal(v.TimeWindow)`), so a round trip that returned
			// `"\"7d\""` or dropped the value would be a real encoding bug.
			expect(version?.timeWindow).toBe("7d");
			expect(version?.includeRecap).toBe(true);
			expect(version?.includePulse).toBe(true);
			expect(version?.sortMode).toBe("relevance");
			expect(version?.supersededBy).toBeUndefined();
		},
	);

	test(
		"GetLens for an unknown lens_id is an empty answer, not an error",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// New coverage. The driver's convention is `(nil, nil)` for a miss
			// and the handler passes that through, so an unknown lens is a 200
			// with an omitted message — not `not_found`. Pinning it matters
			// because the two are handled by completely different branches in
			// a connect-es client, and the SPA's "lens was deleted in another
			// tab" path depends on this one.
			const response = await rpc.post(procedure("GetLens"), { data: { lensId: uuid() } });
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"CreateLens requires created_at",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `created_at` is client-assigned here — like the
			// lens_id — because the immutable data model wants identifiers and
			// timestamps stable across a reproject rather than re-minted from
			// wall clock on every replay. So the handler cannot default it, and
			// rejects instead.
			await expectUnaryError(
				rpc,
				procedure("CreateLens"),
				{
					lens: {
						lensId: uuid(),
						userId: principal.userId,
						tenantId: principal.tenantId,
						name: "no timestamp",
					},
				},
				"invalid_argument",
			);
		},
	);

	test(
		"CreateLensVersion for a lens that does not exist fails the write",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// New coverage, and this one pins **current** behaviour rather
			// than desired behaviour — when it is fixed, this test fails,
			// which is the intended signal.
			//
			// `knowledge_lens_versions.lens_id` has a foreign key into
			// `knowledge_lenses`, so this is a caller error: the client named a
			// lens that is not there. The handler wraps every driver error in
			// `connect.CodeInternal`, so it surfaces as HTTP 500 — a page-an-
			// operator status for a bad request. `failed_precondition` or
			// `not_found` is what a client could actually act on.
			//
			// The assertion is worth having anyway: without it, a handler that
			// dropped the version silently on FK violation would look
			// identical to one that stored it.
			await expectUnaryError(
				rpc,
				procedure("CreateLensVersion"),
				{
					version: {
						lensVersionId: uuid(),
						lensId: uuid(),
						queryText: "orphan",
						createdAt: instant().occurredAt,
					},
				},
				"internal",
			);
		},
	);
});

test.describe("current selection", () => {
	test(
		"SelectCurrentLens then GetCurrentLensSelection round-trips",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// Port of `12-lens-select-current.hurl`, which chained the two
			// calls in one file "so a failure in Select is reported before the
			// Get masks it" — the right instinct, expressed the only way Hurl
			// allowed. Here the seed is a function call and the assertion is
			// about the round trip, so a Select that silently no-ops is caught
			// by the Get rather than hidden by it.
			const seeded = await seedLens(rpc, principal);
			const { occurredAt } = instant();

			const selected = await rpc.post(procedure("SelectCurrentLens"), {
				data: {
					selection: {
						userId: principal.userId,
						lensId: seeded.lensId,
						lensVersionId: seeded.lensVersionId,
						selectedAt: occurredAt,
					},
				},
			});
			expect(selected.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetCurrentLensSelection"), {
					data: { userId: principal.userId },
				}),
				200,
				// `found` is `z.literal(true)` in the schema, not
				// `z.boolean()`: protojson omits a false bool entirely, so
				// "found is present" and "found is true" are the same fact and
				// the schema should say so.
				currentLensSelectionSchema,
			);
			expect(body.selection.userId).toBe(principal.userId);
			expect(body.selection.lensId).toBe(seeded.lensId);
			expect(body.selection.lensVersionId).toBe(seeded.lensVersionId);
		},
	);

	test(
		"SelectCurrentLens is an upsert, not an insert",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `knowledge_current_lens` is keyed on `user_id`
			// alone with `ON CONFLICT (user_id) DO UPDATE`, so selecting a
			// second lens must *replace* the first. Without the upsert the
			// second call would fail on the primary key and a user could never
			// change lens after their first pick — a bug that only ever shows
			// up on the second selection, which no single-shot scenario makes.
			const first = await seedLens(rpc, principal);
			const second = await seedLens(rpc, principal);

			for (const lens of [first, second]) {
				const response = await rpc.post(procedure("SelectCurrentLens"), {
					data: {
						selection: {
							userId: principal.userId,
							lensId: lens.lensId,
							lensVersionId: lens.lensVersionId,
							selectedAt: instant().occurredAt,
						},
					},
				});
				expect(response.status()).toBe(200);
			}

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetCurrentLensSelection"), {
					data: { userId: principal.userId },
				}),
				200,
				currentLensSelectionSchema,
			);
			expect(body.selection.lensId).toBe(second.lensId);
		},
	);

	test(
		"ClearCurrentLens removes the selection",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage — never exercised by the Hurl suite. "No lens
			// selected" is the SPA's default state, so the transition back to
			// it is as load-bearing as the transition into it.
			const seeded = await seedLens(rpc, principal);
			await rpc.post(procedure("SelectCurrentLens"), {
				data: {
					selection: {
						userId: principal.userId,
						lensId: seeded.lensId,
						lensVersionId: seeded.lensVersionId,
						selectedAt: instant().occurredAt,
					},
				},
			});

			const cleared = await rpc.post(procedure("ClearCurrentLens"), {
				data: { userId: principal.userId },
			});
			expect(cleared.status()).toBe(200);

			const response = await rpc.post(procedure("GetCurrentLensSelection"), {
				data: { userId: principal.userId },
			});
			expect(response.status()).toBe(200);
			// `found: false` and a nil selection are both omitted by protojson,
			// so "nothing selected" is the empty document.
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"ResolveLensFilter returns the selected lens's filter",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for the read every Knowledge Home request makes
			// first. With no `lens_id` the driver resolves the user's current
			// selection, then that lens's newest non-superseded version, and
			// projects its filter — three joins where a wrong one silently
			// widens or empties the user's Home rather than erroring.
			const seeded = await seedLens(rpc, principal);
			await rpc.post(procedure("SelectCurrentLens"), {
				data: {
					selection: {
						userId: principal.userId,
						lensId: seeded.lensId,
						lensVersionId: seeded.lensVersionId,
						selectedAt: instant().occurredAt,
					},
				},
			});

			const body = await expectJsonStatus(
				await rpc.post(procedure("ResolveLensFilter"), {
					data: { userId: principal.userId },
				}),
				200,
				resolvedLensFilterSchema,
			);
			expect(body.filter.queryText).toBe("playwright");
			expect(body.filter.tagIds).toEqual(["alpha", "beta"]);
			expect(body.filter.timeWindow).toBe("7d");
			expect(body.filter.sortMode).toBe("relevance");
		},
	);

	test(
		"ResolveLensFilter answers not-found when nothing is selected",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// The control for the test above. `found: false` with no filter is
			// the "unfiltered Home" signal; if this returned a filter, or an
			// error, a user who never chose a lens would get someone's
			// defaults.
			const response = await rpc.post(procedure("ResolveLensFilter"), {
				data: { userId: principal.userId },
			});
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
		},
	);
});

test.describe("archive", () => {
	test(
		"ArchiveLens hides the lens and drops its selection",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage — listed as deferred in the Hurl README. Archiving
			// runs in one transaction that sets `archived_at` *and* deletes the
			// row from `knowledge_current_lens`, and both halves matter: a
			// selection left pointing at an archived lens would resolve to a
			// filter for a lens the user can no longer see or edit.
			const seeded = await seedLens(rpc, principal);
			await rpc.post(procedure("SelectCurrentLens"), {
				data: {
					selection: {
						userId: principal.userId,
						lensId: seeded.lensId,
						lensVersionId: seeded.lensVersionId,
						selectedAt: instant().occurredAt,
					},
				},
			});

			const archived = await rpc.post(procedure("ArchiveLens"), {
				data: { lensId: seeded.lensId },
			});
			expect(archived.status()).toBe(200);

			const listed = await expectJsonStatus(
				await rpc.post(procedure("ListLenses"), { data: { userId: principal.userId } }),
				200,
				listLensesSchema,
			);
			expect(
				listed.lenses ?? [],
				"ListLenses filters on archived_at IS NULL",
			).toEqual([]);

			const selection = await rpc.post(procedure("GetCurrentLensSelection"), {
				data: { userId: principal.userId },
			});
			expect(selection.status()).toBe(200);
			expect(
				await selection.json(),
				"archiving must clear the selection in the same transaction",
			).toEqual({});

			// The lens row itself survives — archive is a soft delete, and
			// `GetLens` is how an audit trail or an undo would reach it.
			const fetched = await expectJsonStatus(
				await rpc.post(procedure("GetLens"), { data: { lensId: seeded.lensId } }),
				200,
				getLensSchema,
			);
			expect(fetched.lens.archivedAt).toBeDefined();
		},
	);
});
