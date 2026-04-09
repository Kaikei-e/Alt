import { describe, it, expect, vi, beforeEach } from "vitest";
import { flushSync } from "svelte";

// Mock modules
vi.mock("$app/paths", () => ({ base: "" }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("$lib/connect/transport-client", () => ({
	createClientTransport: vi.fn(() => ({})),
}));
vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
	Code: { Unauthenticated: 16 },
	ConnectError: class ConnectError extends Error {
		code: number;
		constructor(message: string, code: number) {
			super(message);
			this.code = code;
		}
	},
}));
vi.mock("$lib/gen/alt/search/v2/global_search_pb", () => ({
	GlobalSearchService: {},
}));

import { createClient } from "@connectrpc/connect";
import { useGlobalSearch } from "./useGlobalSearch.svelte.ts";

type SearchResult = ReturnType<typeof useGlobalSearch>;

function createHook(): { search: SearchResult; cleanup: () => void } {
	let search!: SearchResult;
	const cleanup = $effect.root(() => {
		search = useGlobalSearch();
		flushSync();
	});
	return { search, cleanup };
}

describe("useGlobalSearch", () => {
	let mockSearchFn: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		mockSearchFn = vi.fn();
		vi.mocked(createClient).mockReturnValue({
			searchEverything: mockSearchFn,
		} as never);
	});

	it("starts with empty initial state", () => {
		const { search, cleanup } = createHook();

		expect(search.query).toBe("");
		expect(search.loading).toBe(false);
		expect(search.error).toBeNull();
		expect(search.result).toBeNull();
		expect(search.hasResults).toBe(false);
		expect(search.degradedSections).toEqual([]);

		cleanup();
	});

	it("sets loading during search and resolves with result", async () => {
		const mockResponse = {
			query: "test",
			articleSection: {
				hits: [
					{
						id: "a1",
						title: "Test Article",
						snippet: "A <em>test</em> snippet",
						link: "https://example.com/a1",
						tags: ["tech"],
						matchedFields: ["title", "content"],
					},
				],
				estimatedTotal: 10n,
				hasMore: true,
			},
			recapSection: {
				hits: [
					{
						id: "r1",
						jobId: "job-1",
						genre: "Technology",
						summary: "Tech recap summary",
						topTerms: ["AI", "ML"],
						tags: ["tech"],
						windowDays: 7,
						executedAt: "2026-04-01T00:00:00Z",
					},
				],
				estimatedTotal: 3n,
				hasMore: false,
			},
			tagSection: {
				hits: [
					{ tagName: "tech", articleCount: 42 },
					{ tagName: "ai", articleCount: 15 },
				],
				total: 2n,
			},
			degradedSections: [],
			searchedAt: "2026-04-05T10:00:00Z",
		};

		mockSearchFn.mockResolvedValue(mockResponse);
		const { search, cleanup } = createHook();

		const promise = search.search("test");
		expect(search.loading).toBe(true);
		expect(search.query).toBe("test");

		await promise;

		expect(search.loading).toBe(false);
		expect(search.error).toBeNull();
		expect(search.result).not.toBeNull();
		expect(search.hasResults).toBe(true);

		// Verify article section
		expect(search.result?.articleSection?.hits).toHaveLength(1);
		expect(search.result?.articleSection?.hits[0].title).toBe("Test Article");
		expect(search.result?.articleSection?.hasMore).toBe(true);

		// Verify recap section
		expect(search.result?.recapSection?.hits).toHaveLength(1);
		expect(search.result?.recapSection?.hits[0].genre).toBe("Technology");

		// Verify tag section
		expect(search.result?.tagSection?.hits).toHaveLength(2);
		expect(search.result?.tagSection?.hits[0].tagName).toBe("tech");

		cleanup();
	});

	it("handles search errors", async () => {
		mockSearchFn.mockRejectedValue(new Error("Network error"));
		const { search, cleanup } = createHook();

		await search.search("fail");

		expect(search.loading).toBe(false);
		expect(search.error).not.toBeNull();
		expect(search.error?.message).toBe("Network error");
		expect(search.result).toBeNull();
		expect(search.hasResults).toBe(false);

		cleanup();
	});

	it("clears state on clear()", async () => {
		const mockResponse = {
			query: "test",
			articleSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			recapSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			tagSection: { hits: [], total: 0n },
			degradedSections: [],
			searchedAt: "2026-04-05T10:00:00Z",
		};
		mockSearchFn.mockResolvedValue(mockResponse);
		const { search, cleanup } = createHook();

		await search.search("test");
		expect(search.query).toBe("test");

		search.clear();
		expect(search.query).toBe("");
		expect(search.result).toBeNull();
		expect(search.error).toBeNull();
		expect(search.loading).toBe(false);

		cleanup();
	});

	it("reports degraded sections", async () => {
		const mockResponse = {
			query: "test",
			articleSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			recapSection: undefined,
			tagSection: { hits: [], total: 0n },
			degradedSections: ["recaps"],
			searchedAt: "2026-04-05T10:00:00Z",
		};

		mockSearchFn.mockResolvedValue(mockResponse);
		const { search, cleanup } = createHook();

		await search.search("test");

		expect(search.degradedSections).toEqual(["recaps"]);

		cleanup();
	});

	it("ignores empty query", async () => {
		const { search, cleanup } = createHook();

		await search.search("");
		expect(mockSearchFn).not.toHaveBeenCalled();
		expect(search.loading).toBe(false);

		await search.search("   ");
		expect(mockSearchFn).not.toHaveBeenCalled();

		cleanup();
	});

	it("passes optional limits to the RPC", async () => {
		const mockResponse = {
			query: "q",
			articleSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			recapSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			tagSection: { hits: [], total: 0n },
			degradedSections: [],
			searchedAt: "2026-04-05T10:00:00Z",
		};
		mockSearchFn.mockResolvedValue(mockResponse);
		const { search, cleanup } = createHook();

		await search.search("q", { articleLimit: 10, recapLimit: 5, tagLimit: 20 });

		expect(mockSearchFn).toHaveBeenCalledWith({
			query: "q",
			articleLimit: 10,
			recapLimit: 5,
			tagLimit: 20,
		});

		cleanup();
	});

	it("replaces previous result on new search", async () => {
		const response1 = {
			query: "first",
			articleSection: {
				hits: [
					{
						id: "a1",
						title: "First",
						snippet: "",
						link: "",
						tags: [],
						matchedFields: [],
					},
				],
				estimatedTotal: 1n,
				hasMore: false,
			},
			recapSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			tagSection: { hits: [], total: 0n },
			degradedSections: [],
			searchedAt: "2026-04-05T10:00:00Z",
		};
		const response2 = {
			query: "second",
			articleSection: {
				hits: [
					{
						id: "a2",
						title: "Second",
						snippet: "",
						link: "",
						tags: [],
						matchedFields: [],
					},
				],
				estimatedTotal: 1n,
				hasMore: false,
			},
			recapSection: { hits: [], estimatedTotal: 0n, hasMore: false },
			tagSection: { hits: [], total: 0n },
			degradedSections: [],
			searchedAt: "2026-04-05T10:01:00Z",
		};

		mockSearchFn
			.mockResolvedValueOnce(response1)
			.mockResolvedValueOnce(response2);
		const { search, cleanup } = createHook();

		await search.search("first");
		expect(search.result?.articleSection?.hits[0].title).toBe("First");

		await search.search("second");
		expect(search.result?.articleSection?.hits[0].title).toBe("Second");
		expect(search.query).toBe("second");

		cleanup();
	});
});
