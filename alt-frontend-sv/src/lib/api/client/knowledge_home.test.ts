import { beforeEach, describe, expect, it, vi } from "vitest";

const { mockCreateClientTransport, mockTrackHomeAction } = vi.hoisted(() => ({
	mockCreateClientTransport: vi.fn(() => ({})),
	mockTrackHomeAction: vi.fn(),
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: mockCreateClientTransport,
}));

vi.mock("#lib/connect/knowledge_home.js", () => ({
	trackHomeAction: mockTrackHomeAction,
}));

import { trackHomeActionClient } from "./knowledge_home.js";

describe("trackHomeActionClient", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockTrackHomeAction.mockResolvedValue(undefined);
	});

	it("calls trackHomeAction even when running in test mode", async () => {
		await trackHomeActionClient(
			"open",
			"article:123",
			JSON.stringify({ source: "article_page" }),
		);

		expect(mockCreateClientTransport).toHaveBeenCalledTimes(1);
		expect(mockTrackHomeAction).toHaveBeenCalledTimes(1);
		expect(mockTrackHomeAction).toHaveBeenCalledWith(
			expect.anything(),
			"open",
			"article:123",
			JSON.stringify({ source: "article_page" }),
		);
	});

	it("returns early when itemKey is empty", async () => {
		await trackHomeActionClient("open", "");

		expect(mockTrackHomeAction).not.toHaveBeenCalled();
	});

	it("swallows errors from trackHomeAction without rejecting", async () => {
		mockTrackHomeAction.mockRejectedValueOnce(new Error("RPC failed"));

		await expect(
			trackHomeActionClient("open", "article:123"),
		).resolves.toBeUndefined();
	});
});
