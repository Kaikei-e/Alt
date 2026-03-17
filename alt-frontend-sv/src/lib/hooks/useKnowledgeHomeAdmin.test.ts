import { describe, expect, it, vi } from "vitest";
import {
	useKnowledgeHomeAdmin,
	type KnowledgeHomeAdminSnapshot,
	type KnowledgeHomeAdminActionRequest,
} from "./useKnowledgeHomeAdmin.svelte";

const initialSnapshot: KnowledgeHomeAdminSnapshot = {
	health: {
		activeVersion: 1,
		checkpointSeq: 42,
		lastUpdated: "2026-03-18T00:00:00Z",
		backfillJobs: [],
	},
	flags: {
		enableHomePage: true,
		enableTracking: true,
		enableProjectionV2: false,
		rolloutPercentage: 10,
		enableRecallRail: true,
		enableLens: true,
		enableStreamUpdates: true,
		enableSupersedeUx: true,
	},
};

describe("useKnowledgeHomeAdmin", () => {
	it("keeps stale data when refresh fails", async () => {
		const fetcher = vi
			.fn()
			.mockResolvedValueOnce(initialSnapshot)
			.mockRejectedValueOnce(new Error("network failed"));

		const admin = useKnowledgeHomeAdmin(fetcher);

		await admin.fetchData();
		expect(admin.health?.checkpointSeq).toBe(42);

		await admin.fetchData();
		expect(admin.health?.checkpointSeq).toBe(42);
		expect(admin.error?.message).toBe("network failed");
	});

	it("sets refreshing state without clearing existing data", async () => {
		let resolveFetch!: (
			value: KnowledgeHomeAdminSnapshot | PromiseLike<KnowledgeHomeAdminSnapshot>,
		) => void;
		const fetcherImpl = () =>
			new Promise<KnowledgeHomeAdminSnapshot>((resolve) => {
				resolveFetch = resolve;
			});
		const fetcher = vi.fn(fetcherImpl);

		const admin = useKnowledgeHomeAdmin(fetcher);
		admin.seed({
			health: {
				activeVersion: 1,
				checkpointSeq: 5,
				lastUpdated: "2026-03-18T00:00:00Z",
				backfillJobs: [],
			},
			flags: {
				enableHomePage: true,
				enableTracking: false,
				enableProjectionV2: false,
				rolloutPercentage: 0,
				enableRecallRail: false,
				enableLens: false,
				enableStreamUpdates: false,
				enableSupersedeUx: false,
			},
		});

		const pending = admin.fetchData();
		expect(admin.refreshing).toBe(true);
		expect(admin.health?.checkpointSeq).toBe(5);

		resolveFetch({
			health: {
				activeVersion: 2,
				checkpointSeq: 6,
				lastUpdated: "2026-03-18T00:00:10Z",
				backfillJobs: [],
			},
			flags: {
				enableHomePage: true,
				enableTracking: true,
				enableProjectionV2: true,
				rolloutPercentage: 100,
				enableRecallRail: true,
				enableLens: true,
				enableStreamUpdates: true,
				enableSupersedeUx: true,
			},
		});
		await pending;

		expect(admin.refreshing).toBe(false);
		expect(admin.health?.checkpointSeq).toBe(6);
	});

	it("runs backfill actions and refreshes the snapshot", async () => {
		const refreshedSnapshot: KnowledgeHomeAdminSnapshot = {
			health: {
				activeVersion: 2,
				checkpointSeq: 99,
				lastUpdated: "2026-03-18T00:01:00Z",
				backfillJobs: [
					{
						jobId: "job-1",
						status: "running",
						projectionVersion: 2,
						totalEvents: 100,
						processedEvents: 1,
						errorMessage: "",
						createdAt: "2026-03-18T00:00:55Z",
						startedAt: "2026-03-18T00:00:56Z",
						completedAt: "",
					},
				],
			},
			flags: initialSnapshot.flags,
		};
		const fetcher = vi.fn().mockResolvedValue(refreshedSnapshot);
		const actionRunner = vi.fn<
			(action: KnowledgeHomeAdminActionRequest) => Promise<void>
		>().mockResolvedValue(undefined);

		const admin = useKnowledgeHomeAdmin(fetcher, actionRunner);
		admin.seed(initialSnapshot);

		await admin.triggerBackfill(2);

		expect(actionRunner).toHaveBeenCalledWith({
			action: "trigger",
			projectionVersion: 2,
		});
		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(admin.health?.checkpointSeq).toBe(99);
		expect(admin.acting).toBe(false);
	});

	it("keeps stale data when a backfill action fails", async () => {
		const fetcher = vi.fn().mockResolvedValue(initialSnapshot);
		const actionRunner = vi.fn<
			(action: KnowledgeHomeAdminActionRequest) => Promise<void>
		>().mockRejectedValue(new Error("trigger failed"));

		const admin = useKnowledgeHomeAdmin(fetcher, actionRunner);
		admin.seed(initialSnapshot);

		await admin.pauseBackfill("job-1");

		expect(admin.health?.checkpointSeq).toBe(42);
		expect(admin.error?.message).toBe("trigger failed");
		expect(admin.acting).toBe(false);
		expect(admin.activeJobId).toBe(null);
	});
});
