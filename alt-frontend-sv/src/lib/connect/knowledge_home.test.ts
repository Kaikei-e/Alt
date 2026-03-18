import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock the connect module
vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
}));

vi.mock("$lib/gen/alt/knowledge_home/v1/knowledge_home_pb", () => ({
	KnowledgeHomeService: {},
}));

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import {
	getKnowledgeHome,
	trackHomeItemsSeen,
	trackHomeAction,
	createKnowledgeHomeClient,
} from "./knowledge_home";

describe("knowledge_home client", () => {
	let mockTransport: Transport;
	let mockClient: {
		getKnowledgeHome: ReturnType<typeof vi.fn>;
		trackHomeItemsSeen: ReturnType<typeof vi.fn>;
		trackHomeAction: ReturnType<typeof vi.fn>;
	};

	beforeEach(() => {
		mockTransport = {} as Transport;
		mockClient = {
			getKnowledgeHome: vi.fn(),
			trackHomeItemsSeen: vi.fn(),
			trackHomeAction: vi.fn(),
		};
		(createClient as unknown as ReturnType<typeof vi.fn>).mockReturnValue(
			mockClient as never,
		);
	});

	describe("createKnowledgeHomeClient", () => {
		it("creates a client with given transport", () => {
			createKnowledgeHomeClient(mockTransport);
			expect(createClient).toHaveBeenCalledWith(
				expect.anything(),
				mockTransport,
			);
		});
	});

	describe("getKnowledgeHome", () => {
		it("returns converted items and digest", async () => {
			mockClient.getKnowledgeHome.mockResolvedValue({
				todayDigest: {
					date: "2026-03-17",
					newArticles: 42,
					summarizedArticles: 30,
					unsummarizedArticles: 12,
					topTags: ["AI", "Go"],
					weeklyRecapAvailable: true,
					eveningPulseAvailable: false,
					needToKnowCount: 3,
				},
				items: [
					{
						itemKey: "article:abc-123",
						itemType: "article",
						articleId: "abc-123",
						title: "Test Article",
						publishedAt: "2026-03-17T10:00:00Z",
						summaryExcerpt: "A test excerpt",
						summaryState: "ready",
						tags: ["AI", "ML"],
						why: [
							{ code: "new_unread", refId: undefined, tag: undefined },
							{ code: "tag_hotspot", refId: undefined, tag: "AI" },
						],
						score: 0.9,
					},
				],
				nextCursor: "cursor-abc",
				hasMore: true,
				degradedMode: false,
				generatedAt: "2026-03-17T10:05:00Z",
				serviceQuality: "full",
			});

			const result = await getKnowledgeHome(mockTransport);

			expect(mockClient.getKnowledgeHome).toHaveBeenCalledWith({
				cursor: undefined,
				limit: 20,
			});
			expect(result.items).toHaveLength(1);
			expect(result.items[0].itemKey).toBe("article:abc-123");
			expect(result.items[0].why).toHaveLength(2);
			expect(result.items[0].why[0].code).toBe("new_unread");
			expect(result.items[0].why[1].tag).toBe("AI");
			expect(result.items[0].summaryState).toBe("ready");
			expect(result.digest).not.toBeNull();
			expect(result.digest!.newArticles).toBe(42);
			expect(result.digest!.topTags).toEqual(["AI", "Go"]);
			expect(result.digest!.needToKnowCount).toBe(3);
			expect(result.nextCursor).toBe("cursor-abc");
			expect(result.hasMore).toBe(true);
			expect(result.degraded).toBe(false);
			expect(result.serviceQuality).toBe("full");
		});

		it("passes cursor and limit parameters", async () => {
			mockClient.getKnowledgeHome.mockResolvedValue({
				items: [],
				nextCursor: "",
				hasMore: false,
				degradedMode: false,
				generatedAt: "2026-03-17T10:00:00Z",
			});

			await getKnowledgeHome(mockTransport, "cursor-xyz", 50);

			expect(mockClient.getKnowledgeHome).toHaveBeenCalledWith({
				cursor: "cursor-xyz",
				limit: 50,
			});
		});

		it("returns null digest when not present", async () => {
			mockClient.getKnowledgeHome.mockResolvedValue({
				todayDigest: undefined,
				items: [],
				nextCursor: "",
				hasMore: false,
				degradedMode: true,
				generatedAt: "2026-03-17T10:00:00Z",
				serviceQuality: "fallback",
			});

			const result = await getKnowledgeHome(mockTransport);

			expect(result.digest).toBeNull();
			expect(result.degraded).toBe(true);
			expect(result.serviceQuality).toBe("fallback");
		});

		it("falls back to degraded flag when service quality is omitted", async () => {
			mockClient.getKnowledgeHome.mockResolvedValue({
				items: [],
				nextCursor: "",
				hasMore: false,
				degradedMode: true,
				generatedAt: "2026-03-17T10:00:00Z",
			});

			const result = await getKnowledgeHome(mockTransport);

			expect(result.serviceQuality).toBe("degraded");
		});
	});

	describe("trackHomeItemsSeen", () => {
		it("sends item keys and session id", async () => {
			mockClient.trackHomeItemsSeen.mockResolvedValue({});

			await trackHomeItemsSeen(
				mockTransport,
				["article:a", "article:b"],
				"session-123",
			);

			expect(mockClient.trackHomeItemsSeen).toHaveBeenCalledWith({
				itemKeys: ["article:a", "article:b"],
				exposureSessionId: "session-123",
			});
		});
	});

	describe("trackHomeAction", () => {
		it("sends action type and item key", async () => {
			mockClient.trackHomeAction.mockResolvedValue({});

			await trackHomeAction(mockTransport, "open", "article:abc-123");

			expect(mockClient.trackHomeAction).toHaveBeenCalledWith({
				actionType: "open",
				itemKey: "article:abc-123",
				metadataJson: undefined,
			});
		});

		it("sends optional metadata", async () => {
			mockClient.trackHomeAction.mockResolvedValue({});

			await trackHomeAction(
				mockTransport,
				"dismiss",
				"article:abc-123",
				'{"reason":"not_interested"}',
			);

			expect(mockClient.trackHomeAction).toHaveBeenCalledWith({
				actionType: "dismiss",
				itemKey: "article:abc-123",
				metadataJson: '{"reason":"not_interested"}',
			});
		});
	});
});
