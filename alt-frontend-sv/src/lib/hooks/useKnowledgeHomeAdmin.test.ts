import { describe, expect, it, vi } from "vitest";
import {
	useKnowledgeHomeAdmin,
	type KnowledgeHomeAdminSnapshot,
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
			},
		});
		await pending;

		expect(admin.refreshing).toBe(false);
		expect(admin.health?.checkpointSeq).toBe(6);
	});
});
