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
	sloStatus: null,
	reprojectRuns: [],
	systemMetrics: null,
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
			value:
				| KnowledgeHomeAdminSnapshot
				| PromiseLike<KnowledgeHomeAdminSnapshot>,
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
			sloStatus: null,
			reprojectRuns: [],
			systemMetrics: null,
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
			sloStatus: null,
			reprojectRuns: [],
			systemMetrics: null,
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
			sloStatus: null,
			reprojectRuns: [],
			systemMetrics: null,
		};
		const fetcher = vi.fn().mockResolvedValue(refreshedSnapshot);
		const actionRunner = vi
			.fn<(action: KnowledgeHomeAdminActionRequest) => Promise<void>>()
			.mockResolvedValue(undefined);

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

	it("surfaces emit_article_url_backfill counters via urlBackfillResult", async () => {
		const fetcher = vi.fn().mockResolvedValue(initialSnapshot);
		const actionRunner = vi
			.fn<(action: KnowledgeHomeAdminActionRequest) => Promise<void>>()
			.mockResolvedValue(undefined);

		const fetchMock = vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({
				ok: true,
				result: {
					articlesScanned: 12252,
					eventsAppended: 12000,
					skippedBlockedScheme: 252,
					skippedDuplicate: 0,
					moreRemaining: false,
				},
			}),
		});
		vi.stubGlobal("fetch", fetchMock);

		const admin = useKnowledgeHomeAdmin(fetcher, actionRunner);
		admin.seed(initialSnapshot);

		await admin.emitArticleUrlBackfill(0, false);

		expect(fetchMock).toHaveBeenCalledWith(
			"/api/admin/knowledge-home",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify({
					action: "emit_article_url_backfill",
					maxArticles: 0,
					dryRun: false,
				}),
			}),
		);
		expect(admin.urlBackfillResult).toEqual({
			articlesScanned: 12252,
			eventsAppended: 12000,
			skippedBlockedScheme: 252,
			skippedDuplicate: 0,
			moreRemaining: false,
		});
		expect(admin.error).toBe(null);
		expect(admin.acting).toBe(false);

		vi.unstubAllGlobals();
	});

	it("captures the error and leaves prior urlBackfillResult untouched on failure", async () => {
		const fetcher = vi.fn().mockResolvedValue(initialSnapshot);
		const actionRunner = vi
			.fn<(action: KnowledgeHomeAdminActionRequest) => Promise<void>>()
			.mockResolvedValue(undefined);

		const fetchMock = vi.fn().mockResolvedValue({
			ok: false,
			json: async () => ({ error: "Failed to run admin action." }),
		});
		vi.stubGlobal("fetch", fetchMock);

		const admin = useKnowledgeHomeAdmin(fetcher, actionRunner);
		admin.seed(initialSnapshot);

		await admin.emitArticleUrlBackfill(50, true);

		expect(admin.error?.message).toBe("Failed to run admin action.");
		expect(admin.urlBackfillResult).toBe(null);
		expect(admin.acting).toBe(false);

		vi.unstubAllGlobals();
	});

	it("keeps stale data when a backfill action fails", async () => {
		const fetcher = vi.fn().mockResolvedValue(initialSnapshot);
		const actionRunner = vi
			.fn<(action: KnowledgeHomeAdminActionRequest) => Promise<void>>()
			.mockRejectedValue(new Error("trigger failed"));

		const admin = useKnowledgeHomeAdmin(fetcher, actionRunner);
		admin.seed(initialSnapshot);

		await admin.pauseBackfill("job-1");

		expect(admin.health?.checkpointSeq).toBe(42);
		expect(admin.error?.message).toBe("trigger failed");
		expect(admin.acting).toBe(false);
		expect(admin.activeJobId).toBe(null);
	});
});
