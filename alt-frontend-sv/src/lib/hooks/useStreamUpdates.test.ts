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

import { createClient } from "@connectrpc/connect";
import { useStreamUpdates } from "./useStreamUpdates.svelte.ts";

describe("useStreamUpdates (Node / enabled=false)", () => {
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

	it("should not connect when enabled is false", () => {
		const stream = useStreamUpdates({
			get enabled() {
				return false;
			},
			get lensId() {
				return undefined;
			},
		});
		expect(stream.isConnected).toBe(false);
		expect(stream.pendingCount).toBe(0);
		expect(mockStreamFn).not.toHaveBeenCalled();
	});

	it("should expose initial state correctly", () => {
		const stream = useStreamUpdates({
			get enabled() {
				return false;
			},
			get lensId() {
				return undefined;
			},
		});
		expect(stream.isConnected).toBe(false);
		expect(stream.isFallback).toBe(false);
		expect(stream.pendingCount).toBe(0);
		expect(stream.pendingUpdates).toEqual([]);
	});

	it("should handle applyUpdates calling onRefresh", () => {
		const onRefresh = vi.fn();
		const stream = useStreamUpdates({
			get enabled() {
				return false;
			},
			get lensId() {
				return undefined;
			},
			onRefresh,
		});
		const applied = stream.applyUpdates();
		expect(applied).toEqual([]);
		expect(onRefresh).toHaveBeenCalled();
	});

	it("should be a complete no-op when enabled is false", () => {
		const stream = useStreamUpdates({
			get enabled() {
				return false;
			},
			get lensId() {
				return undefined;
			},
		});
		expect(stream.isConnected).toBe(false);
		expect(stream.pendingCount).toBe(0);
		expect(stream.isFallback).toBe(false);
		expect(stream.pendingUpdates).toEqual([]);
		expect(mockStreamFn).not.toHaveBeenCalled();
	});
});
