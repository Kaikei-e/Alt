import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/paths", () => ({ base: "/sv" }));
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn(() => ({ __transport: true })),
}));

const fetchArticleContent = vi.fn();
const batchPrefetchArticleContent = vi.fn();
vi.mock("$lib/connect/articles", () => ({
	fetchArticleContent: (...args: unknown[]) => fetchArticleContent(...args),
	batchPrefetchArticleContent: (...args: unknown[]) =>
		batchPrefetchArticleContent(...args),
}));

import { FETCH_PRIORITY_HEADER } from "$lib/connect/transport-client";
import {
	batchPrefetchArticleContentClient,
	getFeedContentOnTheFlyClient,
} from "./articles";

describe("getFeedContentOnTheFlyClient", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		fetchArticleContent.mockResolvedValue({
			url: "https://a.example/1",
			content: "<p>body</p>",
			articleId: "art-1",
			ogImageUrl: "",
			ogImageProxyUrl: "",
		});
	});

	it("passes the caller's priority down as the transport sentinel", async () => {
		await getFeedContentOnTheFlyClient("https://a.example/1", {
			priority: "high",
		});

		const headers = fetchArticleContent.mock.calls[0]?.[4] as Record<
			string,
			string
		>;
		expect(headers[FETCH_PRIORITY_HEADER]).toBe("high");
	});

	it("defaults to no priority hint when the caller does not ask", async () => {
		await getFeedContentOnTheFlyClient("https://a.example/1");

		expect(fetchArticleContent.mock.calls[0]?.[4]).toBeUndefined();
	});
});

describe("batchPrefetchArticleContentClient", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		batchPrefetchArticleContent.mockResolvedValue({
			acceptedCount: 1,
			shedCount: 0,
			rejectedCount: 0,
			skippedSameHostCount: 0,
		});
	});

	// A warm is never on a path anybody waits on, so it takes the low lane.
	it("warms at low fetch priority", async () => {
		await batchPrefetchArticleContentClient(["https://a.example/1"]);

		expect(batchPrefetchArticleContent).toHaveBeenCalledWith(
			expect.anything(),
			["https://a.example/1"],
			expect.objectContaining({ [FETCH_PRIORITY_HEADER]: "low" }),
		);
	});
});
