/**
 * Feed API Contract Tests
 *
 * Validates that mock data used in E2E tests conforms to the proto schema.
 * This prevents mock drift: when the proto changes, these tests break before
 * the E2E tests silently pass with stale mock data.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	GetUnreadFeedsResponseSchema,
	GetAllFeedsResponseSchema,
	GetFeedStatsResponseSchema,
	GetDetailedFeedStatsResponseSchema,
	MarkAsReadResponseSchema,
	type FeedItem,
} from "$lib/gen/alt/feeds/v2/feeds_pb";
import {
	buildConnectFeedsResponse,
	buildConnectFeedItem,
} from "../../../tests/e2e/fixtures/factories";

describe("Feed API Contract", () => {
	it("GetUnreadFeedsResponse conforms to proto schema", () => {
		const mockData = buildConnectFeedsResponse();
		const response = create(GetUnreadFeedsResponseSchema, {
			data: mockData.data.map((f) => ({
				id: f.id,
				title: f.title,
				description: f.description,
				link: f.link,
				published: f.published,
				createdAt: f.createdAt,
				author: f.author,
				articleId: f.articleId,
			})),
			hasMore: mockData.hasMore,
		});

		expect(response.data).toHaveLength(2);
		expect(response.data[0].title).toBe("AI Trends");
		expect(response.hasMore).toBe(false);
	});

	it("GetAllFeedsResponse round-trips through proto serialization", () => {
		const original = create(GetAllFeedsResponseSchema, {
			data: [
				{
					id: "feed-1",
					title: "Test Feed",
					description: "Test",
					link: "https://example.com",
					published: "1 hour ago",
					createdAt: new Date().toISOString(),
					author: "Author",
				},
			],
			hasMore: false,
		});

		const binary = toBinary(GetAllFeedsResponseSchema, original);
		const deserialized = fromBinary(GetAllFeedsResponseSchema, binary);

		expect(deserialized.data).toHaveLength(original.data.length);
		expect(deserialized.data[0].title).toBe("Test Feed");
		expect(deserialized.hasMore).toBe(false);
	});

	it("FeedItem has required fields", () => {
		const feedItem = buildConnectFeedItem({
			title: "Required Fields Test",
		});

		// All these fields should be present in a well-formed FeedItem
		expect(feedItem.id).toBeDefined();
		expect(feedItem.title).toBeDefined();
		expect(feedItem.description).toBeDefined();
		expect(feedItem.link).toBeDefined();
		expect(feedItem.published).toBeDefined();
		expect(feedItem.createdAt).toBeDefined();
		expect(feedItem.author).toBeDefined();
	});

	it("GetFeedStatsResponse uses bigint for counts", () => {
		const response = create(GetFeedStatsResponseSchema, {
			feedAmount: 10n,
			summarizedFeedAmount: 5n,
		});

		expect(typeof response.feedAmount).toBe("bigint");
		expect(response.feedAmount).toBe(10n);
	});

	it("GetDetailedFeedStatsResponse includes all stat fields", () => {
		const response = create(GetDetailedFeedStatsResponseSchema, {
			feedAmount: 12n,
			articleAmount: 345n,
			unsummarizedFeedAmount: 7n,
		});

		expect(response.feedAmount).toBe(12n);
		expect(response.articleAmount).toBe(345n);
		expect(response.unsummarizedFeedAmount).toBe(7n);
	});

	it("MarkAsReadResponse has message field", () => {
		const response = create(MarkAsReadResponseSchema, {
			message: "Feed marked as read",
		});

		expect(response.message).toBe("Feed marked as read");
	});
});
