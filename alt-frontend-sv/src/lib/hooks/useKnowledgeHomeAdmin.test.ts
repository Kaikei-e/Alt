import { describe, expect, it, vi } from "vitest";
import { useKnowledgeHomeAdmin } from "./useKnowledgeHomeAdmin.svelte";

describe("useKnowledgeHomeAdmin", () => {
	it("keeps stale data when refresh fails", async () => {
		const fetcher = vi
			.fn()
			.mockResolvedValueOnce({
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
			})
			.mockRejectedValueOnce(new Error("network failed"));

		const admin = useKnowledgeHomeAdmin(fetcher);

		await admin.fetchData();
		expect(admin.health?.checkpointSeq).toBe(42);

		await admin.fetchData();
		expect(admin.health?.checkpointSeq).toBe(42);
		expect(admin.error?.message).toBe("network failed");
	});

	it("sets refreshing state without clearing existing data", async () => {
		let resolveFetch: ((value: Awaited<ReturnType<typeof Promise.resolve>>) => void) | null =
			null;
		const fetcher = vi.fn(
			() =>
				new Promise((resolve) => {
					resolveFetch = resolve as typeof resolveFetch;
				}),
		);

		const admin = useKnowledgeHomeAdmin(fetcher as () => Promise<never>);
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

		resolveFetch?.({
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
