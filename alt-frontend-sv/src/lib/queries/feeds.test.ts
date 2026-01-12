/**
 * Feed Queries Tests
 *
 * Tests for TanStack Query helper functions
 */
import { describe, expect, it } from "vitest";
import { flattenFeedPages, flattenSearchPages } from "./feeds";
import type { FeedCursorResponse, FeedSearchResponse, ConnectFeedItem } from "$lib/connect/feeds";

// Mock feed item
const createMockFeedItem = (id: string): ConnectFeedItem => ({
	id,
	title: `Feed ${id}`,
	link: `https://example.com/${id}`,
	description: `Description ${id}`,
	published: new Date().toISOString(),
	createdAt: new Date().toISOString(),
	author: "Author",
	articleId: undefined,
});

describe("flattenFeedPages", () => {
	it("should return empty array when data is undefined", () => {
		const result = flattenFeedPages(undefined);
		expect(result).toEqual([]);
	});

	it("should return empty array when pages is undefined", () => {
		const result = flattenFeedPages({ pages: undefined } as any);
		expect(result).toEqual([]);
	});

	it("should return empty array when pages is empty", () => {
		const result = flattenFeedPages({ pages: [] });
		expect(result).toEqual([]);
	});

	it("should flatten single page correctly", () => {
		const feed1 = createMockFeedItem("1");
		const feed2 = createMockFeedItem("2");

		const data = {
			pages: [
				{
					data: [feed1, feed2],
					hasMore: false,
					nextCursor: null,
				} as FeedCursorResponse,
			],
		};

		const result = flattenFeedPages(data);
		expect(result).toHaveLength(2);
		expect(result[0].id).toBe("1");
		expect(result[1].id).toBe("2");
	});

	it("should flatten multiple pages correctly", () => {
		const feed1 = createMockFeedItem("1");
		const feed2 = createMockFeedItem("2");
		const feed3 = createMockFeedItem("3");
		const feed4 = createMockFeedItem("4");

		const data = {
			pages: [
				{
					data: [feed1, feed2],
					hasMore: true,
					nextCursor: "cursor-1",
				} as FeedCursorResponse,
				{
					data: [feed3, feed4],
					hasMore: false,
					nextCursor: null,
				} as FeedCursorResponse,
			],
		};

		const result = flattenFeedPages(data);
		expect(result).toHaveLength(4);
		expect(result.map((f) => f.id)).toEqual(["1", "2", "3", "4"]);
	});

	it("should handle pages with empty data arrays", () => {
		const feed1 = createMockFeedItem("1");

		const data = {
			pages: [
				{
					data: [feed1],
					hasMore: true,
					nextCursor: "cursor-1",
				} as FeedCursorResponse,
				{
					data: [],
					hasMore: false,
					nextCursor: null,
				} as FeedCursorResponse,
			],
		};

		const result = flattenFeedPages(data);
		expect(result).toHaveLength(1);
		expect(result[0].id).toBe("1");
	});
});

describe("flattenSearchPages", () => {
	it("should return empty array when data is undefined", () => {
		const result = flattenSearchPages(undefined);
		expect(result).toEqual([]);
	});

	it("should return empty array when pages is undefined", () => {
		const result = flattenSearchPages({ pages: undefined } as any);
		expect(result).toEqual([]);
	});

	it("should return empty array when pages is empty", () => {
		const result = flattenSearchPages({ pages: [] });
		expect(result).toEqual([]);
	});

	it("should flatten single search page correctly", () => {
		const feed1 = createMockFeedItem("1");
		const feed2 = createMockFeedItem("2");

		const data = {
			pages: [
				{
					data: [feed1, feed2],
					hasMore: false,
					nextCursor: null,
					total: 2,
				} as FeedSearchResponse,
			],
		};

		const result = flattenSearchPages(data);
		expect(result).toHaveLength(2);
		expect(result[0].id).toBe("1");
		expect(result[1].id).toBe("2");
	});

	it("should flatten multiple search pages correctly", () => {
		const feed1 = createMockFeedItem("1");
		const feed2 = createMockFeedItem("2");
		const feed3 = createMockFeedItem("3");

		const data = {
			pages: [
				{
					data: [feed1, feed2],
					hasMore: true,
					nextCursor: 2,
					total: 3,
				} as FeedSearchResponse,
				{
					data: [feed3],
					hasMore: false,
					nextCursor: null,
					total: 3,
				} as FeedSearchResponse,
			],
		};

		const result = flattenSearchPages(data);
		expect(result).toHaveLength(3);
		expect(result.map((f) => f.id)).toEqual(["1", "2", "3"]);
	});
});

describe("feedKeys", () => {
	it("should have correct key structure", async () => {
		const { feedKeys } = await import("./keys");

		expect(feedKeys.all).toEqual(["feeds"]);
		expect(feedKeys.lists()).toEqual(["feeds", "list"]);
		expect(feedKeys.unread()).toEqual(["feeds", "list", { filter: "unread" }]);
		expect(feedKeys.read()).toEqual(["feeds", "list", { filter: "read" }]);
		expect(feedKeys.favorites()).toEqual(["feeds", "list", { filter: "favorites" }]);
		expect(feedKeys.search("test")).toEqual(["feeds", "search", "test"]);
		expect(feedKeys.stats()).toEqual(["feeds", "stats"]);
		expect(feedKeys.detailedStats()).toEqual(["feeds", "stats", "detailed"]);
		expect(feedKeys.unreadCount()).toEqual(["feeds", "unreadCount"]);
	});
});
