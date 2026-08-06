import type { APIRequestContext } from "@playwright/test";
import { expectJsonStatus } from "../../_shared/http.js";
import { uuid } from "../../_shared/ids.js";
import { appendEvent, instant } from "./fixtures.js";
import type { Principal } from "./fixtures.js";
import { snapshotMetadataSchema } from "./schemas.js";
import type { z } from "zod";

/**
 * Seeding for the `/admin/*` specs.
 *
 * `SnapshotHandler.CreateSnapshot` refuses outright when `GetMaxEventSeq`
 * returns 0 ("no events found, cannot create snapshot"), and
 * `RetentionHandler.RunRetention` refuses when there is no valid snapshot. So
 * both admin surfaces have a precondition that lives in a *different* service
 * port from the one under test.
 *
 * The Hurl suite satisfied those preconditions with file ordering: scenario 03
 * appended the event, 14 made the snapshot, and 17 ran retention — which is
 * one of the two reasons its `run.sh` passed `--jobs 1`. Making each test seed
 * its own chain removes the ordering entirely, so the admin specs are
 * independent of each other and of the rest of the suite; the `workers: 1`
 * project they run in exists only to keep two *snapshot writers* from racing
 * `/admin/snapshots/latest`, not to restore an order.
 */

export type SeededSnapshot = {
	/** The snapshot as the create endpoint reported it. */
	readonly metadata: z.infer<typeof snapshotMetadataSchema>;
	/** `event_seq` of the event appended to satisfy the `max_event_seq > 0` gate. */
	readonly eventSeq: string;
};

export async function seedSnapshot(
	rpc: APIRequestContext,
	admin: APIRequestContext,
	principal: Principal,
): Promise<SeededSnapshot> {
	const { occurredAt } = instant();
	const eventSeq = await appendEvent(rpc, {
		tenantId: principal.tenantId,
		userId: principal.userId,
		// An event type no projector folds: the point is only to move
		// `max(event_seq)` above zero, and a foldable type would make this
		// helper race the projectors for no benefit.
		eventType: "E2ESnapshotBoundary",
		aggregateId: `article:${uuid()}`,
		dedupeKey: `${principal.token}-snapshot-boundary`,
		occurredAt,
	});

	const metadata = await expectJsonStatus(
		await admin.post("/admin/snapshots/create"),
		200,
		snapshotMetadataSchema,
	);

	return { metadata, eventSeq };
}
