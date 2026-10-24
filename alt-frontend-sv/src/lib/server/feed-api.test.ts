import { beforeEach, describe, expect, it, vi } from "vitest";

const {
	callBackendAPI,
	callBackendAPIWithBody,
	createServerTransport,
	createServerTransportWithToken,
	getUnreadFeedsConnect,
	getReadFeedsConnect,
} = vi.hoisted(() => ({
	callBackendAPI: vi.fn(),
	callBackendAPIWithBody: vi.fn(),
	createServerTransport: vi.fn(),
	createServerTransportWithToken: vi.fn(),
	getUnreadFeedsConnect: vi.fn(),
	getReadFeedsConnect: vi.fn(),
}));

vi.mock("./backend-rest-client", () => ({
	callBackendAPI,
	callBackendAPIWithBody,
}));

vi.mock("#lib/connect/transport-server.js", () => ({
	createServerTransport,
	createServerTransportWithToken,
}));

vi.mock("#lib/connect/feeds.js", () => ({
	getUnreadFeeds: getUnreadFeedsConnect,
	getReadFeeds: getReadFeedsConnect,
}));

import {
	deleteFeedLink,
	getFeedLinks,
	getFeedStats,
	getFeedsWithCursor,
	getReadFeedsWithCursor,
	getTodayUnreadCount,
	registerRssFeed,
	updateFeedReadStatus,
} from "./feed-api";

describe("feed-api", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("forwards backendToken in updateFeedReadStatus", async () => {
		callBackendAPIWithBody.mockResolvedValue(undefined);

		await updateFeedReadStatus(
			"cookie=1",
			"https://example.com/feed.xml",
			"tok-123",
		);

		expect(callBackendAPIWithBody).toHaveBeenCalledWith(
			"/v1/feeds/read",
			"cookie=1",
			"POST",
			{ feed_url: "https://example.com/feed.xml" },
			{ backendToken: "tok-123" },
		);
	});

	it("forwards backendToken in getFeedStats and getTodayUnreadCount", async () => {
		callBackendAPI.mockResolvedValue({});

		await getFeedStats("cookie=1", "tok-stats");
		expect(callBackendAPI).toHaveBeenCalledWith(
			"/v1/feeds/stats/detailed",
			"cookie=1",
			{ backendToken: "tok-stats" },
		);

		await getTodayUnreadCount("cookie=1", "2026-04-07T00:00:00Z", "tok-unread");
		expect(callBackendAPI).toHaveBeenCalledWith(
			"/v1/feeds/count/unreads?since=2026-04-07T00%3A00%3A00Z",
			"cookie=1",
			{ backendToken: "tok-unread" },
		);
	});

	it("forwards backendToken in getFeedLinks, registerRssFeed, and deleteFeedLink", async () => {
		callBackendAPI.mockResolvedValue([
			{
				id: "link-1",
				url: "https://example.com/rss",
				health_status: "healthy",
				consecutive_failures: 0,
				is_active: true,
				last_failure_reason: null,
			},
		]);
		const fakeFetch = vi.fn() as unknown as typeof fetch;

		const links = await getFeedLinks("cookie=1", fakeFetch, "tok-links");
		expect(callBackendAPI).toHaveBeenCalledWith(
			"/v1/rss-feed-link/list",
			"cookie=1",
			{ fetch: fakeFetch, backendToken: "tok-links" },
		);
		expect(links).toEqual([
			{
				id: "link-1",
				url: "https://example.com/rss",
				healthStatus: "healthy",
				consecutiveFailures: 0,
				isActive: true,
				lastFailureReason: "",
			},
		]);

		await registerRssFeed("cookie=1", "https://example.com/rss", "tok-reg");
		expect(callBackendAPIWithBody).toHaveBeenCalledWith(
			"/v1/rss-feed-link/register",
			"cookie=1",
			"POST",
			{ url: "https://example.com/rss" },
			{ backendToken: "tok-reg" },
		);

		await deleteFeedLink("cookie=1", "id/with/slash", "tok-del");
		expect(callBackendAPIWithBody).toHaveBeenCalledWith(
			"/v1/rss-feed-link/id%2Fwith%2Fslash",
			"cookie=1",
			"DELETE",
			undefined,
			{ backendToken: "tok-del" },
		);
	});

	it("uses createServerTransportWithToken when backendToken is provided in getFeedsWithCursor and getReadFeedsWithCursor", async () => {
		const fakeTransport = {};
		createServerTransportWithToken.mockReturnValue(fakeTransport);
		getUnreadFeedsConnect.mockResolvedValue({
			data: [
				{
					id: "1",
					title: "Item 1",
					description: "Desc",
					link: "https://example.com/1",
					published: "2 hours ago",
					createdAt: "2026-04-07T00:00:00Z",
					author: "Alice",
					articleId: "art-1",
					feedId: "feed-1",
					ogImageProxyUrl: "https://proxy/img",
				},
			],
			nextCursor: "cur-2",
			hasMore: true,
		});
		getReadFeedsConnect.mockResolvedValue({
			data: [],
			nextCursor: null,
			hasMore: false,
		});

		const unread = await getFeedsWithCursor(
			"cookie=1",
			"cur-1",
			10,
			"tok-cursor",
		);
		expect(createServerTransportWithToken).toHaveBeenCalledWith(
			"tok-cursor",
			undefined,
		);
		expect(createServerTransport).not.toHaveBeenCalled();
		expect(unread.next_cursor).toBe("cur-2");
		expect(unread.data[0]?.article_id).toBe("art-1");

		await getReadFeedsWithCursor("cookie=1", undefined, 20, "tok-read");
		expect(createServerTransportWithToken).toHaveBeenCalledWith(
			"tok-read",
			undefined,
		);
	});
});
