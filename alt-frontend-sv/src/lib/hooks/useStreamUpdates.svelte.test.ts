import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { flushSync } from "svelte";

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

import { createClient } from "@connectrpc/connect";
import { useStreamUpdates } from "./useStreamUpdates.svelte.ts";
import {
	createReactiveFlag,
	createReactiveString,
} from "./test-helpers/effect-root.svelte.ts";

type StreamResult = ReturnType<typeof useStreamUpdates>;

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

/**
 * Mock stream that delivers events then hangs forever (never closes).
 * Simulates a real long-lived SSE stream, preventing reconnection noise.
 */
function createHangingMockStream(events: Array<Record<string, unknown>>) {
	let index = 0;
	return {
		[Symbol.asyncIterator]() {
			return {
				next(): Promise<IteratorResult<Record<string, unknown>>> {
					if (index < events.length) {
						return Promise.resolve({ value: events[index++], done: false });
					}
					// Hang forever — stream stays open
					return new Promise(() => {});
				},
			};
		},
	};
}

/** Wait for real timers. */
function wait(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Wait for a reactive assertion to pass inside a $effect.
 * The assertion is re-evaluated whenever any $state it reads changes.
 * Uses real timers — COALESCE_DELAY (3s) must actually elapse.
 */
function waitForEffect(
	assertion: () => void,
	timeoutMs = 8000,
): Promise<void> {
	let cleanupRoot: (() => void) | null = null;

	const promise = new Promise<void>((resolve, reject) => {
		const timer = setTimeout(() => {
			try {
				assertion();
				resolve();
			} catch (e) {
				reject(e);
			}
		}, timeoutMs);

		cleanupRoot = $effect.root(() => {
			$effect(() => {
				try {
					assertion();
					clearTimeout(timer);
					resolve();
				} catch {
					// Assertion not yet satisfied — wait for next reactive update
				}
			});
		});
	});

	return promise.finally(() => cleanupRoot?.());
}

/**
 * Remove BroadcastChannel for the entire test duration.
 * Returns a restore function to call at test end.
 */
function disableBroadcastChannel(): () => void {
	const orig = globalThis.BroadcastChannel;
	// @ts-expect-error - testing without BroadcastChannel
	delete globalThis.BroadcastChannel;
	return () => {
		globalThis.BroadcastChannel = orig;
	};
}

/**
 * Create hook inside $effect.root with flushSync inside the root callback
 * (per Svelte docs pattern). Returns the hook result and cleanup function.
 */
function createHook(opts: Parameters<typeof useStreamUpdates>[0]): {
	stream: StreamResult;
	cleanup: () => void;
} {
	let stream!: StreamResult;
	const cleanup = $effect.root(() => {
		stream = useStreamUpdates(opts);
		flushSync();
	});
	return { stream, cleanup };
}

describe("useStreamUpdates (Browser / $effect)", () => {
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

	// ── BroadcastChannel / leader election tests ──

	it("gracefully degrades when BroadcastChannel unavailable", () => {
		const restore = disableBroadcastChannel();
		mockStreamFn.mockReturnValue(createMockStream([]));

		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		expect(stream.isLeader).toBe(true);
		expect(mockStreamFn).toHaveBeenCalled();

		cleanup();
		restore();
	});

	it("uses BroadcastChannel for leader election when available", () => {
		vi.useFakeTimers();
		try {
			mockStreamFn.mockReturnValue(createMockStream([]));

			const { stream, cleanup } = createHook({
				get enabled() { return true; },
				get lensId() { return undefined; },
			});

			expect(stream.isLeader).toBe(false);
			expect(mockStreamFn).not.toHaveBeenCalled();

			vi.advanceTimersByTime(3100);
			expect(stream.isLeader).toBe(true);

			cleanup();
		} finally {
			vi.useRealTimers();
		}
	});

	it("registers visibilitychange listener when enabled", () => {
		const restore = disableBroadcastChannel();
		const addListenerSpy = vi.spyOn(document, "addEventListener");
		mockStreamFn.mockReturnValue(createMockStream([]));

		const { cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		const visibilityCalls = addListenerSpy.mock.calls.filter(
			(call) => call[0] === "visibilitychange",
		);
		expect(visibilityCalls.length).toBeGreaterThan(0);

		addListenerSpy.mockRestore();
		cleanup();
		restore();
	});

	// ── Stream event processing tests ──

	it("should filter heartbeat events", { timeout: 10000 }, async () => {
		const restore = disableBroadcastChannel();
		const events = [
			{ eventType: "heartbeat", occurredAt: "2026-03-18T12:00:00Z" },
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:1" },
			},
		];
		mockStreamFn.mockReturnValue(createHangingMockStream(events));

		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		await waitForEffect(() => {
			expect(stream.pendingCount).toBe(1);
		});
		expect(stream.pendingUpdates[0].eventType).toBe("item_added");

		cleanup();
		restore();
	});

	it("should coalesce updates within delay window", { timeout: 10000 }, async () => {
		const restore = disableBroadcastChannel();
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
		mockStreamFn.mockReturnValue(createHangingMockStream(events));

		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		await waitForEffect(() => {
			expect(stream.pendingCount).toBe(2);
		});

		cleanup();
		restore();
	});

	it("should handle stream_expired", async () => {
		const restore = disableBroadcastChannel();
		const events = [
			{
				eventType: "stream_expired",
				occurredAt: "2026-03-18T12:00:00Z",
				reconnectAfterMs: 6000,
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		await wait(100);

		expect(stream.pendingCount).toBe(0);
		expect(stream.isConnected).toBe(false);

		cleanup();
		restore();
	});

	it("should handle fallback_to_unary event", { timeout: 10000 }, async () => {
		const restore = disableBroadcastChannel();
		const events = [
			{
				eventType: "fallback_to_unary",
				occurredAt: "2026-03-18T12:00:00Z",
				reconnectAfterMs: 10000,
			},
		];
		mockStreamFn.mockReturnValue(createMockStream(events));

		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		await waitForEffect(() => {
			expect(stream.isFallback).toBe(true);
		});
		expect(stream.isConnected).toBe(false);

		cleanup();
		restore();
	});

	it("should include item data in pending updates", { timeout: 10000 }, async () => {
		const restore = disableBroadcastChannel();
		const events = [
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:abc-123" },
			},
		];
		mockStreamFn.mockReturnValue(createHangingMockStream(events));

		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
		});

		await waitForEffect(() => {
			expect(stream.pendingCount).toBe(1);
		});
		expect(stream.pendingUpdates[0].item?.itemKey).toBe("article:abc-123");

		cleanup();
		restore();
	});

	it("should applyUpdates clearing pending and calling onRefresh", { timeout: 10000 }, async () => {
		const restore = disableBroadcastChannel();
		const events = [
			{
				eventType: "item_added",
				occurredAt: "2026-03-18T12:00:01Z",
				item: { itemKey: "article:1" },
			},
		];
		mockStreamFn.mockReturnValue(createHangingMockStream(events));

		const onRefresh = vi.fn();
		const { stream, cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return undefined; },
			onRefresh,
		});

		await waitForEffect(() => {
			expect(stream.pendingCount).toBe(1);
		});
		const applied = stream.applyUpdates();
		expect(applied).toHaveLength(1);
		expect(applied[0].eventType).toBe("item_added");
		expect(stream.pendingCount).toBe(0);
		expect(onRefresh).toHaveBeenCalled();

		cleanup();
		restore();
	});

	// ── Reactive toggle tests ──

	it("should connect when enabled changes from false to true", () => {
		const restore = disableBroadcastChannel();
		mockStreamFn.mockReturnValue(createMockStream([]));
		const flag = createReactiveFlag(false);

		const { stream, cleanup } = createHook({
			get enabled() { return flag.value; },
			get lensId() { return undefined; },
		});

		expect(mockStreamFn).not.toHaveBeenCalled();

		mockStreamFn.mockReturnValue(createMockStream([]));
		flag.value = true;
		flushSync();

		expect(stream.isLeader).toBe(true);
		expect(mockStreamFn).toHaveBeenCalled();

		cleanup();
		restore();
	});

	it("should disconnect when enabled changes from true to false", () => {
		const restore = disableBroadcastChannel();
		mockStreamFn.mockReturnValue(createMockStream([]));
		const flag = createReactiveFlag(true);

		const { stream, cleanup } = createHook({
			get enabled() { return flag.value; },
			get lensId() { return undefined; },
		});

		expect(stream.isLeader).toBe(true);

		flag.value = false;
		flushSync();

		expect(stream.isConnected).toBe(false);

		cleanup();
		restore();
	});

	it("should reconnect when lensId changes", () => {
		const restore = disableBroadcastChannel();
		mockStreamFn.mockReturnValue(createMockStream([]));
		const lensId = createReactiveString("lens-a");

		const { cleanup } = createHook({
			get enabled() { return true; },
			get lensId() { return lensId.value; },
		});

		expect(mockStreamFn).toHaveBeenCalledTimes(1);

		mockStreamFn.mockReturnValue(createMockStream([]));
		lensId.value = "lens-b";
		flushSync();

		expect(mockStreamFn).toHaveBeenCalledTimes(2);

		cleanup();
		restore();
	});
});
