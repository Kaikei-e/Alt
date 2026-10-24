import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("#lib/api/client/core.js", () => ({
	getClientCSRFToken: vi.fn().mockResolvedValue("test-csrf-token"),
}));

import type { SovereignAdminSnapshot } from "#lib/types/sovereign-admin.js";
import {
	type SovereignAdminActionRequest,
	useSovereignAdmin,
} from "./useSovereignAdmin.svelte";

const sampleSnapshot: SovereignAdminSnapshot = {
	storageStats: [
		{
			table_name: "knowledge_events",
			row_count: 15000,
			total_size: "12 MB",
			total_bytes: 12582912,
			is_partitioned: true,
		},
	],
	snapshots: [
		{
			snapshotId: "snap-1",
			snapshotType: "full",
			projectionVersion: 2,
			projectorBuildRef: "abc1234",
			schemaVersion: "1",
			snapshotAt: "2026-03-20T00:00:00Z",
			eventSeqBoundary: 9000,
			snapshotDataPath: "/snapshots/snap-1",
			itemsRowCount: 500,
			itemsChecksum: "sha256:111",
			digestRowCount: 30,
			digestChecksum: "sha256:222",
			recallRowCount: 40,
			recallChecksum: "sha256:333",
			createdAt: "2026-03-20T00:00:00Z",
			status: "completed",
		},
	],
	latestSnapshot: null,
	retentionLogs: [],
	eligiblePartitions: [],
};

describe("useSovereignAdmin", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("fetches sovereign storage metrics and snapshots via fetchData()", async () => {
		const fetcher = vi.fn().mockResolvedValueOnce(sampleSnapshot);
		const admin = useSovereignAdmin(fetcher);

		await admin.fetchData();

		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(admin.storageStats).toEqual(sampleSnapshot.storageStats);
		expect(admin.snapshots).toEqual(sampleSnapshot.snapshots);
		expect(admin.error).toBeNull();
		expect(admin.refreshing).toBe(false);
	});

	it("triggers create_snapshot via createSnapshot() and refreshes snapshot data", async () => {
		const fetcher = vi.fn().mockResolvedValue(sampleSnapshot);
		const actionRunner = vi
			.fn<(action: SovereignAdminActionRequest) => Promise<unknown>>()
			.mockResolvedValue({});

		const admin = useSovereignAdmin(fetcher, actionRunner);
		await admin.createSnapshot();

		expect(actionRunner).toHaveBeenCalledWith({ action: "create_snapshot" });
		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(admin.acting).toBe(false);
		expect(admin.error).toBeNull();
	});

	it("runs retention in both dry-run and live modes via runRetention()", async () => {
		const fetcher = vi.fn().mockResolvedValue(sampleSnapshot);
		const actionRunner = vi.fn().mockResolvedValue({});

		const dryRunResponse = {
			dry_run: true,
			actions: [
				{
					action: "archive",
					table: "knowledge_events",
					partition: "knowledge_events_2025_01",
					rows: 1000,
					status: "dry_run",
				},
			],
		};
		const liveResponse = {
			dry_run: false,
			actions: [
				{
					action: "archive",
					table: "knowledge_events",
					partition: "knowledge_events_2025_01",
					rows: 1000,
					status: "completed",
				},
			],
		};

		const fetchMock = vi
			.fn()
			.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ result: dryRunResponse }),
			})
			.mockResolvedValueOnce({
				ok: true,
				json: async () => ({ result: liveResponse }),
			});
		vi.stubGlobal("fetch", fetchMock);

		const admin = useSovereignAdmin(fetcher, actionRunner);

		await admin.runRetention(true);
		expect(fetchMock).toHaveBeenNthCalledWith(
			1,
			"/api/admin/knowledge-home/sovereign",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify({ action: "run_retention", dry_run: true }),
			}),
		);
		expect(admin.retentionResult).toEqual(dryRunResponse);

		await admin.runRetention(false);
		expect(fetchMock).toHaveBeenNthCalledWith(
			2,
			"/api/admin/knowledge-home/sovereign",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify({ action: "run_retention", dry_run: false }),
			}),
		);
		expect(admin.retentionResult).toEqual(liveResponse);
		expect(fetcher).toHaveBeenCalledTimes(2);

		vi.unstubAllGlobals();
	});

	it("preserves existing state and records error when fetchData or runRetention fails", async () => {
		const fetcher = vi
			.fn()
			.mockResolvedValueOnce(sampleSnapshot)
			.mockRejectedValueOnce(new Error("Sovereign metrics unreachable"));
		const actionRunner = vi.fn().mockResolvedValue({});

		const admin = useSovereignAdmin(fetcher, actionRunner);
		await admin.fetchData();
		expect(admin.storageStats).toHaveLength(1);

		await admin.fetchData();
		expect(admin.storageStats).toHaveLength(1);
		expect(admin.error?.message).toBe("Sovereign metrics unreachable");

		const fetchMock = vi.fn().mockResolvedValueOnce({
			ok: false,
			json: async () => ({ error: "Retention lock held" }),
		});
		vi.stubGlobal("fetch", fetchMock);

		await admin.runRetention(true);
		expect(admin.error?.message).toBe("Retention lock held");
		expect(admin.acting).toBe(false);

		vi.unstubAllGlobals();
	});
});
