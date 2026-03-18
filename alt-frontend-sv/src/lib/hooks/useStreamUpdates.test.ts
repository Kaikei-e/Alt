import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock modules
vi.mock("$app/paths", () => ({ base: "" }));
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("$lib/connect/transport-client", () => ({
	createClientTransport: vi.fn(() => ({})),
}));
vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
}));
vi.mock("$lib/gen/alt/knowledge_home/v1/knowledge_home_pb", () => ({
	KnowledgeHomeService: {},
}));
vi.mock("svelte", () => ({
	onDestroy: vi.fn(),
}));

import { createClient } from "@connectrpc/connect";
import { useStreamUpdates } from "./useStreamUpdates.svelte.ts";

function createMockStream(events: Array<Record<string, unknown>>) {
	let index = 0;
	return {
		[Symbol.asyncIterator]() {
			return {
				async next() {
					if (index < events.length) {
						return { value: events[index++], done: false };
					}
					return { value: undefined, done: true };
				},
			};
		},
	};
}

/** Wait for real timers. Used in stream-processing tests (no fake timers). */
function wait(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Temporarily remove BroadcastChannel so the hook connects directly
 * (no leader election delay), then restore after callback.
 */
function withoutBroadcastChannel<T>(fn: () => T): T {
	const orig = globalThis.BroadcastChannel;
	// @ts-expect-error - testing without BroadcastChannel
	delete globalThis.BroadcastChannel;
	try {
		return fn();
	} finally {
		globalThis.BroadcastChannel = orig;
	}
}

describe("useStreamUpdates", () => {
	let mockStreamFn: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		mockStreamFn = vi.fn();
		vi.mocked(createClient).mockReturnValue({
			streamKnowledgeHomeUpdates: mockStreamFn,
		} as never);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	// ── State management tests (synchronous, no stream needed) ──

	it("should not connect when enabled is false", () => {
		const stream = useStreamUpdates({ enabled: false });
		expect(stream.isConnected).toBe(false);
		expect(stream.pendingCount).toBe(0);
		expect(mockStreamFn).not.toHaveBeenCalled();
	});

	it("should expose initial state correctly", () => {
		const stream = useStreamUpdates({ enabled: false });
		expect(stream.isConnected).toBe(false);
		expect(stream.isFallback).toBe(false);
		expect(stream.pendingCount).toBe(0);
		expect(stream.pendingUpdates).toEqual([]);
	});

	it("should handle applyUpdates calling onRefresh", () => {
		const onRefresh = vi.fn();
		const stream = useStreamUpdates({ enabled: false, onRefresh });
		const applied = stream.applyUpdates();
		expect(applied).toEqual([]);
		expect(onRefresh).toHaveBeenCalled();
	});

	it("should be a complete no-op when enabled is false", () => {
		const stream = useStreamUpdates({ enabled: false });
		expect(stream.isConnected).toBe(false);
		expect(stream.pendingCount).toBe(0);
		expect(stream.isFallback).toBe(false);
		expect(stream.pendingUpdates).toEqual([]);
		expect(mockStreamFn).not.toHaveBeenCalled();
	});

	// ── BroadcastChannel / leader election tests (fake timers OK) ──

	it("gracefully degrades when BroadcastChannel unavailable", () => {
		mockStreamFn.mockReturnValue(createMockStream([]));
		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true }),
		);
		// Without BroadcastChannel, becomes leader immediately and connects
		expect(stream.isLeader).toBe(true);
		expect(mockStreamFn).toHaveBeenCalled();
	});

	it("uses BroadcastChannel for leader election when available", () => {
		vi.useFakeTimers();
		try {
			mockStreamFn.mockReturnValue(createMockStream([]));
			const stream = useStreamUpdates({ enabled: true });

			// Before claim timeout, not yet leader
			expect(stream.isLeader).toBe(false);
			expect(mockStreamFn).not.toHaveBeenCalled();

			// After LEADER_CLAIM_TIMEOUT (3000ms), becomes leader
			vi.advanceTimersByTime(3100);
			expect(stream.isLeader).toBe(true);
		} finally {
			vi.useRealTimers();
		}
	});

	it("registers visibilitychange listener when enabled", () => {
		if (typeof globalThis.document === "undefined") return;

		const addListenerSpy = vi.spyOn(document, "addEventListener");
		mockStreamFn.mockReturnValue(createMockStream([]));
		withoutBroadcastChannel(() => useStreamUpdates({ enabled: true }));

		const visibilityCalls = addListenerSpy.mock.calls.filter(
			(call) => call[0] === "visibilitychange",
		);
		expect(visibilityCalls.length).toBeGreaterThan(0);

		addListenerSpy.mockRestore();
	});

	// ── Stream event processing tests (real timers, no BroadcastChannel) ──
	// Best practice: test async iterator processing with real timers,
	// bypassing BroadcastChannel leader election for direct connect.

	it("should filter heartbeat events", async () => {
		const events = [
			{ eventType: "heartbeat", occurredAt: "2026-03-18T12:00:00Z" },
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:1" },
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true }),
		);

		// Wait for async iteration + coalesce timer (3000ms)
		await wait(3200);

		expect(stream.pendingCount).toBe(1);
		expect(stream.pendingUpdates[0].eventType).toBe("item_added");
	});

	it("should coalesce updates within delay window", async () => {
		const events = [
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:1" },
			},
			{
				eventType: "item_updated",
				occurredAt: "2026-03-18T12:00:02Z",
				item: { itemKey: "article:2" },
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true }),
		);

		await wait(3200);
		expect(stream.pendingCount).toBe(2);
	});

	it("should handle stream_expired", async () => {
		const events = [
			{
				eventType: "stream_expired",
				occurredAt: "2026-03-18T12:00:00Z",
				reconnectAfterMs: 6000,
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true }),
		);

		await wait(50);

		expect(stream.pendingCount).toBe(0);
		expect(stream.isConnected).toBe(false);
	});

	it("should handle fallback_to_unary event", async () => {
		const events = [
			{
				eventType: "fallback_to_unary",
				occurredAt: "2026-03-18T12:00:00Z",
				reconnectAfterMs: 10000,
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true }),
		);

		await wait(50);

		expect(stream.isFallback).toBe(true);
		expect(stream.isConnected).toBe(false);
	});

	it("should include item data in pending updates", async () => {
		const events = [
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:abc-123" },
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true }),
		);

		await wait(3200);

		expect(stream.pendingCount).toBe(1);
		expect(stream.pendingUpdates[0].item?.itemKey).toBe("article:abc-123");
	});

	it("should applyUpdates clearing pending and calling onRefresh", async () => {
		const events = [
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:1" },
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const onRefresh = vi.fn();
		const stream = withoutBroadcastChannel(() =>
			useStreamUpdates({ enabled: true, onRefresh }),
		);

		await wait(3200);

		expect(stream.pendingCount).toBe(1);
		const applied = stream.applyUpdates();
		expect(applied).toHaveLength(1);
		expect(applied[0].eventType).toBe("item_added");
		expect(stream.pendingCount).toBe(0);
		expect(onRefresh).toHaveBeenCalled();
	});
});
