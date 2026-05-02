import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@connectrpc/connect", () => {
	return {
		createClient: vi.fn(),
	};
});

vi.mock("$lib/gen/alt/feeds/v2/feeds_pb", () => ({
	FeedService: {},
}));

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import {
	getUnreadFeeds,
	getReadFeeds,
	getAllFeeds,
	getFavoriteFeeds,
} from "./listing";
import { listSubscriptions } from "./actions";

const mockedCreateClient = vi.mocked(createClient);

describe("feeds listing — Connect-Timeout-Ms enforcement", () => {
	let transport: Transport;
	let unaryClient: {
		getUnreadFeeds: ReturnType<typeof vi.fn>;
		getReadFeeds: ReturnType<typeof vi.fn>;
		getAllFeeds: ReturnType<typeof vi.fn>;
		getFavoriteFeeds: ReturnType<typeof vi.fn>;
		listSubscriptions: ReturnType<typeof vi.fn>;
	};

	beforeEach(() => {
		transport = {} as Transport;
		const emptyResp = { data: [], nextCursor: undefined, hasMore: false };
		unaryClient = {
			getUnreadFeeds: vi.fn().mockResolvedValue(emptyResp),
			getReadFeeds: vi.fn().mockResolvedValue(emptyResp),
			getAllFeeds: vi.fn().mockResolvedValue(emptyResp),
			getFavoriteFeeds: vi.fn().mockResolvedValue(emptyResp),
			listSubscriptions: vi.fn().mockResolvedValue({ sources: [] }),
		};
		mockedCreateClient.mockReturnValue(unaryClient as never);
	});

	it("getUnreadFeeds passes timeoutMs: 5000 to the unary call", async () => {
		await getUnreadFeeds(transport);
		expect(unaryClient.getUnreadFeeds).toHaveBeenCalledTimes(1);
		const callOptions = unaryClient.getUnreadFeeds.mock.calls[0][1];
		expect(callOptions).toMatchObject({ timeoutMs: 5000 });
	});

	it("getReadFeeds passes timeoutMs: 5000 to the unary call", async () => {
		await getReadFeeds(transport);
		const callOptions = unaryClient.getReadFeeds.mock.calls[0][1];
		expect(callOptions).toMatchObject({ timeoutMs: 5000 });
	});

	it("getAllFeeds passes timeoutMs: 5000 to the unary call", async () => {
		await getAllFeeds(transport);
		const callOptions = unaryClient.getAllFeeds.mock.calls[0][1];
		expect(callOptions).toMatchObject({ timeoutMs: 5000 });
	});

	it("getFavoriteFeeds passes timeoutMs: 5000 to the unary call", async () => {
		await getFavoriteFeeds(transport);
		const callOptions = unaryClient.getFavoriteFeeds.mock.calls[0][1];
		expect(callOptions).toMatchObject({ timeoutMs: 5000 });
	});

	it("listSubscriptions passes timeoutMs: 5000 to the unary call", async () => {
		await listSubscriptions(transport);
		const callOptions = unaryClient.listSubscriptions.mock.calls[0][1];
		expect(callOptions).toMatchObject({ timeoutMs: 5000 });
	});
});
