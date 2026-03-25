import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({ transport: true })),
	getKnowledgeHome: vi.fn(),
	trackHomeItemsSeen: vi.fn(),
	trackHomeAction: vi.fn(),
}));

import { getKnowledgeHome, trackHomeAction } from "$lib/connect";
import { useKnowledgeHome } from "./useKnowledgeHome.svelte";

describe("useKnowledgeHome", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("dismissItem only removes the item locally and does not track by itself", () => {
		const home = useKnowledgeHome();
		home.fetchData = vi.fn() as typeof home.fetchData;

		(home.items as Array<{ itemKey: string; title: string }>).push(
			{ itemKey: "article:1", title: "One" },
			{ itemKey: "article:2", title: "Two" },
		);

		home.dismissItem("article:1");

		expect(home.items.map((item) => item.itemKey)).toEqual(["article:2"]);
		expect(trackHomeAction).not.toHaveBeenCalled();
	});

	it("starts with initial_loading pageState", () => {
		const home = useKnowledgeHome();
		expect(home.pageState).toBe("initial_loading");
	});

	it("transitions to ready after successful fetchData", async () => {
		const mockGetKnowledgeHome = vi.mocked(getKnowledgeHome);
		mockGetKnowledgeHome.mockResolvedValue({
			items: [],
			digest: null,
			hasMore: false,
			degraded: false,
			nextCursor: "",
			serviceQuality: "full",
			featureFlags: [],
			recallCandidates: [],
			generatedAt: "2026-03-17T00:00:00Z",
		});

		const home = useKnowledgeHome();
		await home.fetchData(true);

		expect(home.pageState).toBe("ready");
	});

	it("transitions to degraded when serviceQuality is degraded", async () => {
		const mockGetKnowledgeHome = vi.mocked(getKnowledgeHome);
		mockGetKnowledgeHome.mockResolvedValue({
			items: [],
			digest: null,
			hasMore: false,
			degraded: true,
			nextCursor: "",
			serviceQuality: "degraded",
			featureFlags: [],
			recallCandidates: [],
			generatedAt: "2026-03-17T00:00:00Z",
		});

		const home = useKnowledgeHome();
		await home.fetchData(true);

		expect(home.pageState).toBe("degraded");
	});

	it("transitions to fallback when serviceQuality is fallback", async () => {
		const mockGetKnowledgeHome = vi.mocked(getKnowledgeHome);
		mockGetKnowledgeHome.mockResolvedValue({
			items: [],
			digest: null,
			hasMore: false,
			degraded: false,
			nextCursor: "",
			serviceQuality: "fallback",
			featureFlags: [],
			recallCandidates: [],
			generatedAt: "2026-03-17T00:00:00Z",
		});

		const home = useKnowledgeHome();
		await home.fetchData(true);

		expect(home.pageState).toBe("fallback");
	});

	it("transitions to hard_error when fetchData throws", async () => {
		const mockGetKnowledgeHome = vi.mocked(getKnowledgeHome);
		mockGetKnowledgeHome.mockRejectedValue(new Error("network error"));

		const home = useKnowledgeHome();
		await home.fetchData(true);

		expect(home.pageState).toBe("hard_error");
	});

	it("transitions to refreshing during non-initial fetch", async () => {
		const mockGetKnowledgeHome = vi.mocked(getKnowledgeHome);
		mockGetKnowledgeHome.mockResolvedValue({
			items: [
				{
					itemKey: "article:1",
					itemType: "article",
					title: "Test",
					publishedAt: "2026-03-17T00:00:00Z",
					summaryState: "ready" as const,
					tags: [],
					why: [],
					score: 0.5,
				},
			],
			digest: null,
			hasMore: false,
			degraded: false,
			nextCursor: "",
			serviceQuality: "full",
			featureFlags: [],
			recallCandidates: [],
			generatedAt: "2026-03-17T00:00:00Z",
		});

		const home = useKnowledgeHome();
		// First fetch to reach ready state
		await home.fetchData(true);
		expect(home.pageState).toBe("ready");

		// Second fetch should be refreshing during load
		const promise = home.fetchData(true);
		expect(home.pageState).toBe("refreshing");
		await promise;
		expect(home.pageState).toBe("ready");
	});

	it("exposes emptyReason based on state", async () => {
		const mockGetKnowledgeHome = vi.mocked(getKnowledgeHome);
		mockGetKnowledgeHome.mockResolvedValue({
			items: [],
			digest: null,
			hasMore: false,
			degraded: false,
			nextCursor: "",
			serviceQuality: "full",
			featureFlags: [],
			recallCandidates: [],
			generatedAt: "2026-03-17T00:00:00Z",
		});

		const home = useKnowledgeHome();
		await home.fetchData(true);

		expect(home.emptyReason).toBe("no_data");
	});
});
