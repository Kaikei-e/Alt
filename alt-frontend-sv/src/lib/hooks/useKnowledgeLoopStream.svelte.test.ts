import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { flushSync } from "svelte";

vi.mock("$app/paths", () => ({ base: "" }));
vi.mock("$lib/connect/transport-client", () => ({
	createClientTransport: vi.fn(() => ({})),
}));
vi.mock("@connectrpc/connect", async () => {
	const actual = await vi.importActual<
		typeof import("@connectrpc/connect")
	>("@connectrpc/connect");
	return {
		...actual,
		createClient: vi.fn(),
	};
});

import { createClient } from "@connectrpc/connect";
import { useKnowledgeLoopStream } from "./useKnowledgeLoopStream.svelte.ts";
import {
	createReactiveFlag,
	createReactiveString,
} from "./test-helpers/effect-root.svelte.ts";

type StreamResult = ReturnType<typeof useKnowledgeLoopStream>;

function createHangingMockStream(events: Array<Record<string, unknown>>) {
	let index = 0;
	return {
		[Symbol.asyncIterator]() {
			return {
				next(): Promise<IteratorResult<Record<string, unknown>>> {
					if (index < events.length) {
						return Promise.resolve({ value: events[index++], done: false });
					}
					return new Promise(() => {});
				},
			};
		},
	};
}

/**
 * Stream that immediately throws an AbortError once aborted.
 * Used to model the real Connect-RPC behaviour where AbortController.abort()
 * causes the for-await loop to reject with an AbortError.
 */
function createAbortAwareMockStream(signal: AbortSignal) {
	return {
		[Symbol.asyncIterator]() {
			return {
				next(): Promise<IteratorResult<Record<string, unknown>>> {
					if (signal.aborted) {
						const err = new Error("aborted");
						err.name = "AbortError";
						return Promise.reject(err);
					}
					return new Promise((_resolve, reject) => {
						signal.addEventListener(
							"abort",
							() => {
								const err = new Error("aborted");
								err.name = "AbortError";
								reject(err);
							},
							{ once: true },
						);
					});
				},
			};
		},
	};
}

function createHook(opts: Parameters<typeof useKnowledgeLoopStream>[0]): {
	stream: StreamResult;
	cleanup: () => void;
} {
	let stream!: StreamResult;
	const cleanup = $effect.root(() => {
		stream = useKnowledgeLoopStream(opts);
		flushSync();
	});
	return { stream, cleanup };
}

function wait(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

describe("useKnowledgeLoopStream — lifecycle", () => {
	let mockStreamFn: ReturnType<typeof vi.fn>;
	let capturedSignal: AbortSignal | undefined;

	beforeEach(() => {
		capturedSignal = undefined;
		mockStreamFn = vi.fn((_req, opts: { signal: AbortSignal }) => {
			capturedSignal = opts.signal;
			return createHangingMockStream([]);
		});
		vi.mocked(createClient).mockReturnValue({
			streamKnowledgeLoopUpdates: mockStreamFn,
		} as never);
		try {
			sessionStorage.clear();
		} catch {
			// sessionStorage unavailable in some test envs
		}
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("opens exactly one stream on mount", () => {
		const { cleanup } = createHook({
			get enabled() {
				return true;
			},
			get lensModeId() {
				return "default";
			},
		});
		expect(mockStreamFn).toHaveBeenCalledTimes(1);
		cleanup();
	});

	it(
		"does not schedule a phantom reconnect when the effect tears down (intentional abort)",
		{ timeout: 6000 },
		async () => {
			const enabled = createReactiveFlag(true);
			const { cleanup } = createHook({
				get enabled() {
					return enabled.value;
				},
				get lensModeId() {
					return "default";
				},
			});

			expect(mockStreamFn).toHaveBeenCalledTimes(1);

			mockStreamFn.mockImplementationOnce(
				(_req, opts: { signal: AbortSignal }) => {
					return createAbortAwareMockStream(opts.signal);
				},
			);

			// Tear the effect down. This must abort the current stream and NOT
			// reopen another one — the prior connect()'s catch block was the
			// source of the phantom reconnect that produced overlapping SSE
			// sessions in production (see ADR knowledge-loop reactive nebula).
			enabled.value = false;
			flushSync();

			// Allow scheduleReconnect's debounce window (BASE_RETRY_DELAY_MS = 1s)
			// plus a margin for the catch path to flush.
			await wait(1500);

			expect(mockStreamFn).toHaveBeenCalledTimes(1);
			cleanup();
		},
	);

	it(
		"closes the abort-aware stream when the effect cleanup runs",
		{ timeout: 4000 },
		async () => {
			let abortedFromHook = false;
			mockStreamFn.mockImplementation(
				(_req, opts: { signal: AbortSignal }) => {
					opts.signal.addEventListener("abort", () => {
						abortedFromHook = true;
					});
					return createAbortAwareMockStream(opts.signal);
				},
			);
			const enabled = createReactiveFlag(true);
			const { cleanup } = createHook({
				get enabled() {
					return enabled.value;
				},
				get lensModeId() {
					return "default";
				},
			});

			expect(mockStreamFn).toHaveBeenCalledTimes(1);

			enabled.value = false;
			flushSync();
			await wait(50);

			expect(abortedFromHook).toBe(true);
			cleanup();
		},
	);

	it(
		"resumes from a saved cursor across an effect-driven remount",
		{ timeout: 4000 },
		async () => {
			const lensMode = createReactiveString("default");
			const seenResumeValues: bigint[] = [];
			mockStreamFn.mockImplementation(
				(req: { resumeFromSeq?: bigint }, opts: { signal: AbortSignal }) => {
					seenResumeValues.push(req.resumeFromSeq ?? 0n);
					return createHangingMockStream([]);
				},
			);

			const { cleanup: cleanupA } = createHook({
				get enabled() {
					return true;
				},
				get lensModeId() {
					return lensMode.value ?? "default";
				},
				cursorPersistKey: "user-A:default",
			});
			// Simulate a frame arriving that bumps the high-water mark.
			// The hook must persist the cursor (sessionStorage) so a later
			// remount can resume.
			sessionStorage.setItem(
				"knowledge-loop:resume:user-A:default",
				"42",
			);
			cleanupA();

			// Fresh mount: should resume from 42, not 0.
			const { cleanup: cleanupB } = createHook({
				get enabled() {
					return true;
				},
				get lensModeId() {
					return lensMode.value ?? "default";
				},
				cursorPersistKey: "user-A:default",
			});
			flushSync();

			expect(seenResumeValues).toEqual([0n, 42n]);
			cleanupB();
		},
	);
});
