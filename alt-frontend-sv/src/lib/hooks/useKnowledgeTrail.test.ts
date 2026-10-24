import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: vi.fn(() => ({ transport: true })),
}));

vi.mock("#lib/utils/uuidv7.js", () => ({
	uuidv7: vi.fn(() => "01900000-0000-7000-8000-000000000001"),
}));

vi.mock("#lib/connect/knowledge_trail.js", () => ({
	getTrail: vi.fn(),
	searchTrail: vi.fn(),
	resolveBranch: vi.fn(),
}));

import {
	type EpisodeData,
	type FootprintData,
	getTrail,
	resolveBranch,
	searchTrail,
} from "#lib/connect/knowledge_trail.js";
import { goto } from "$app/navigation";
import { useKnowledgeTrail } from "./useKnowledgeTrail.svelte";

function makeFootprint(itemKey: string, title = "Sample"): FootprintData {
	return {
		footprintKey: `fp-${itemKey}`,
		verb: "read",
		itemKey,
		title,
		excerpt: "Excerpt",
		tags: ["rust"],
		note: "",
		occurredAt: "2026-03-20T10:00:00Z",
		wear: "worn",
		contactCount: 2,
		firstOccurredAt: "2026-03-19T10:00:00Z",
	};
}

function makeEpisode(episodeKey: string, itemKey: string): EpisodeData {
	return {
		episodeKey,
		wear: "worn",
		thumbnailUrl: "",
		footprints: [makeFootprint(itemKey, `Title for ${itemKey}`)],
	};
}

describe("useKnowledgeTrail", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("fetches initial trail episodes, footprints, and branches via fetchData(true)", async () => {
		const ep1 = makeEpisode("ep-1", "article:1");
		vi.mocked(getTrail).mockResolvedValueOnce({
			footprints: ep1.footprints,
			episodes: [ep1],
			branches: [
				{
					branchKey: "br-1",
					anchorItemKey: "article:1",
					relationKind: "continuation",
					why: "Follows up",
					evidenceRefs: [],
					confidence: "high",
					targetItemKey: "article:2",
					targetTitle: "Next Article",
				},
			],
			nextCursor: "cursor-1",
			hasMore: true,
		});

		const trail = useKnowledgeTrail();
		await trail.fetchData(true);

		expect(getTrail).toHaveBeenCalledWith(expect.anything(), undefined);
		expect(trail.episodes).toEqual([ep1]);
		expect(trail.footprints).toEqual(ep1.footprints);
		expect(trail.branches).toHaveLength(1);
		expect(trail.hasMore).toBe(true);
		expect(trail.hasEverLoaded).toBe(true);
		expect(trail.error).toBeNull();
	});

	it("appends episodes on loadMore() using nextCursor and skips when hasMore is false", async () => {
		const ep1 = makeEpisode("ep-1", "article:1");
		const ep2 = makeEpisode("ep-2", "article:2");

		vi.mocked(getTrail)
			.mockResolvedValueOnce({
				footprints: ep1.footprints,
				episodes: [ep1],
				branches: [],
				nextCursor: "cursor-page-2",
				hasMore: true,
			})
			.mockResolvedValueOnce({
				footprints: ep2.footprints,
				episodes: [ep2],
				branches: [],
				nextCursor: "",
				hasMore: false,
			});

		const trail = useKnowledgeTrail();
		await trail.fetchData(true);
		await trail.loadMore();

		expect(getTrail).toHaveBeenNthCalledWith(
			2,
			expect.anything(),
			"cursor-page-2",
		);
		expect(trail.episodes.map((e) => e.episodeKey)).toEqual(["ep-1", "ep-2"]);
		expect(trail.hasMore).toBe(false);

		// Subsequent loadMore() should be a no-op since hasMore is false
		await trail.loadMore();
		expect(getTrail).toHaveBeenCalledTimes(2);
	});

	it("preserves base episodes during search() so clearSearch() restores the spine without refetching (D25)", async () => {
		const ep1 = makeEpisode("ep-1", "article:1");
		const searchHitEp = makeEpisode("ep-hit", "article:99");

		vi.mocked(getTrail).mockResolvedValueOnce({
			footprints: ep1.footprints,
			episodes: [ep1],
			branches: [],
			nextCursor: "",
			hasMore: false,
		});
		vi.mocked(searchTrail).mockResolvedValueOnce({
			episodes: [searchHitEp],
			matchedItemKeys: ["article:99"],
		});

		const trail = useKnowledgeTrail();
		await trail.fetchData(true);
		expect(trail.episodes).toEqual([ep1]);

		// Empty/whitespace query is a no-op
		await trail.search("   ");
		expect(searchTrail).not.toHaveBeenCalled();

		await trail.search("  compilers  ");
		expect(searchTrail).toHaveBeenCalledWith(expect.anything(), "compilers");
		expect(trail.searchActive).toBe(true);
		expect(trail.searchQuery).toBe("compilers");
		expect(trail.searchEpisodes).toEqual([searchHitEp]);
		expect(trail.matchedItemKeys).toEqual(["article:99"]);
		// Base spine episodes remain preserved in memory
		expect(trail.episodes).toEqual([ep1]);

		trail.clearSearch();
		expect(trail.searchActive).toBe(false);
		expect(trail.searchQuery).toBe("");
		expect(trail.searchEpisodes).toEqual([]);
		expect(trail.matchedItemKeys).toEqual([]);
		expect(trail.episodes).toEqual([ep1]);
		// No additional getTrail call was made on clearSearch()
		expect(getTrail).toHaveBeenCalledTimes(1);
	});

	it("records searchError when searchTrail rejects and clears it on clearSearch()", async () => {
		vi.mocked(searchTrail).mockRejectedValueOnce(
			new Error("Search index offline"),
		);

		const trail = useKnowledgeTrail();
		await trail.search("distributed systems");

		expect(trail.searchActive).toBe(false);
		expect(trail.searching).toBe(false);
		expect(trail.searchError?.message).toBe("Search index offline");

		trail.clearSearch();
		expect(trail.searchError).toBeNull();
	});

	it("navigates to the target article with ?trail_proposal= when resolveBranch is called with taken", async () => {
		vi.mocked(resolveBranch).mockResolvedValueOnce(undefined);

		const trail = useKnowledgeTrail();
		await trail.resolveBranch("branch-abc", "taken", "article:art-42");

		expect(resolveBranch).toHaveBeenCalledWith(
			expect.anything(),
			"branch-abc",
			"taken",
			"01900000-0000-7000-8000-000000000001",
			undefined,
		);
		expect(goto).toHaveBeenCalledWith(
			"/articles/art-42?trail_proposal=branch-abc",
		);
		expect(getTrail).not.toHaveBeenCalled();
	});

	it("refetches the trail without navigating when resolveBranch is called with dismissed", async () => {
		vi.mocked(resolveBranch).mockResolvedValueOnce(undefined);
		vi.mocked(getTrail).mockResolvedValueOnce({
			footprints: [],
			episodes: [],
			branches: [],
			nextCursor: "",
			hasMore: false,
		});

		const trail = useKnowledgeTrail();
		await trail.resolveBranch(
			"branch-abc",
			"dismissed",
			undefined,
			"already_known",
		);

		expect(resolveBranch).toHaveBeenCalledWith(
			expect.anything(),
			"branch-abc",
			"dismissed",
			"01900000-0000-7000-8000-000000000001",
			"already_known",
		);
		expect(goto).not.toHaveBeenCalled();
		expect(getTrail).toHaveBeenCalledWith(expect.anything(), undefined);
	});
});
