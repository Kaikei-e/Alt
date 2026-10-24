import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: vi.fn(() => ({ transport: true })),
	listAugurConversations: vi.fn(),
	deleteAugurConversation: vi.fn(),
}));

import {
	type AugurConversationSummary,
	deleteAugurConversation,
	listAugurConversations,
} from "#lib/connect/index.js";
import { useAugurHistory } from "./useAugurHistory.svelte";

function makeSummary(
	id: string,
	title = `Conversation ${id}`,
	messageCount = 2,
): AugurConversationSummary {
	return {
		id,
		title,
		createdAt: new Date("2026-03-20T09:00:00Z"),
		lastActivityAt: new Date("2026-03-20T09:05:00Z"),
		lastMessagePreview: `Preview for ${id}`,
		messageCount,
	};
}

describe("useAugurHistory", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("replaces conversations and updates hasMore on refresh()", async () => {
		const conv1 = makeSummary("c-1");
		const conv2 = makeSummary("c-2");
		vi.mocked(listAugurConversations).mockResolvedValueOnce({
			conversations: [conv1, conv2],
			nextPageToken: "token-page-2",
		});

		const history = useAugurHistory({ pageSize: 10 });
		expect(history.conversations).toEqual([]);
		expect(history.hasMore).toBe(false);

		await history.refresh();

		expect(listAugurConversations).toHaveBeenCalledWith(expect.anything(), {
			pageSize: 10,
		});
		expect(history.conversations).toEqual([conv1, conv2]);
		expect(history.hasMore).toBe(true);
		expect(history.isLoading).toBe(false);
		expect(history.errorMessage).toBe("");
	});

	it("appends next page on loadMore() when nextPageToken is present", async () => {
		const conv1 = makeSummary("c-1");
		const conv2 = makeSummary("c-2");

		vi.mocked(listAugurConversations).mockResolvedValueOnce({
			conversations: [conv2],
			nextPageToken: "",
		});

		const history = useAugurHistory({
			pageSize: 15,
			initialConversations: [conv1],
			initialNextPageToken: "next-token-1",
		});

		expect(history.hasMore).toBe(true);
		await history.loadMore();

		expect(listAugurConversations).toHaveBeenCalledWith(expect.anything(), {
			pageSize: 15,
			pageToken: "next-token-1",
		});
		expect(history.conversations.map((c) => c.id)).toEqual(["c-1", "c-2"]);
		expect(history.hasMore).toBe(false);

		// Calling loadMore() again when nextPageToken is empty is a no-op
		await history.loadMore();
		expect(listAugurConversations).toHaveBeenCalledTimes(1);
	});

	it("optimistically removes a conversation on remove(id) and rolls back if deleteAugurConversation throws", async () => {
		const conv1 = makeSummary("c-1");
		const conv2 = makeSummary("c-2");

		let rejectDelete!: (err: Error) => void;
		vi.mocked(deleteAugurConversation).mockImplementationOnce(
			() =>
				new Promise<void>((_resolve, reject) => {
					rejectDelete = reject;
				}),
		);

		const history = useAugurHistory({
			initialConversations: [conv1, conv2],
		});

		const removePromise = history.remove("c-1");
		// Optimistically removed before the RPC settles
		expect(history.conversations.map((c) => c.id)).toEqual(["c-2"]);

		rejectDelete(new Error("Delete RPC failed"));
		await expect(removePromise).rejects.toThrow("Delete RPC failed");

		// Rolled back to original snapshot and errorMessage set
		expect(history.conversations.map((c) => c.id)).toEqual(["c-1", "c-2"]);
		expect(history.errorMessage).toBe("Delete RPC failed");
	});
});
