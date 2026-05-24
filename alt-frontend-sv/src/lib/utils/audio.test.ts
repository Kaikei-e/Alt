import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createSeamlessTtsPlayer, splitTextForTts } from "./audio";

interface MockSource {
	buffer: AudioBuffer | null;
	connect: ReturnType<typeof vi.fn>;
	start: ReturnType<typeof vi.fn>;
	stop: ReturnType<typeof vi.fn>;
	onended: (() => void) | null;
}

interface MockContext {
	currentTime: number;
	state: AudioContextState;
	destination: AudioDestinationNode;
	decodeAudioData: ReturnType<typeof vi.fn>;
	createBufferSource: ReturnType<typeof vi.fn>;
	close: ReturnType<typeof vi.fn>;
}

describe("createSeamlessTtsPlayer", () => {
	let mockCtx: MockContext;
	let sources: MockSource[];
	const originalAudioContext = (
		globalThis as unknown as { AudioContext?: unknown }
	).AudioContext;

	beforeEach(() => {
		sources = [];
		mockCtx = {
			currentTime: 0,
			state: "running" as AudioContextState,
			destination: {} as AudioDestinationNode,
			decodeAudioData: vi.fn(
				async (bytes: ArrayBuffer) =>
					({ duration: bytes.byteLength * 0.001 }) as AudioBuffer,
			),
			createBufferSource: vi.fn(() => {
				const src: MockSource = {
					buffer: null,
					connect: vi.fn(),
					start: vi.fn(),
					stop: vi.fn(),
					onended: null,
				};
				sources.push(src);
				return src as unknown as AudioBufferSourceNode;
			}),
			close: vi.fn(async () => {
				mockCtx.state = "closed" as AudioContextState;
			}),
		};

		class MockAudioContext {
			constructor() {
				return mockCtx as unknown as AudioContext;
			}
		}
		(globalThis as unknown as { AudioContext: unknown }).AudioContext =
			MockAudioContext as unknown;
	});

	afterEach(() => {
		if (originalAudioContext === undefined) {
			delete (globalThis as unknown as { AudioContext?: unknown }).AudioContext;
		} else {
			(globalThis as unknown as { AudioContext: unknown }).AudioContext =
				originalAudioContext;
		}
	});

	it("throws when AudioContext is not available", () => {
		delete (globalThis as unknown as { AudioContext?: unknown }).AudioContext;
		expect(() => createSeamlessTtsPlayer()).toThrow(
			"Web Audio API (AudioContext) is not available",
		);
	});

	it("rejects empty audio data", async () => {
		const player = createSeamlessTtsPlayer();
		await expect(player.append(new Uint8Array(0))).rejects.toThrow(
			"Empty audio data",
		);
	});

	it("decodes and schedules the first chunk at currentTime + primer", async () => {
		mockCtx.currentTime = 1.0;
		const player = createSeamlessTtsPlayer();
		await player.append(new Uint8Array([1, 2, 3, 4]));

		expect(mockCtx.decodeAudioData).toHaveBeenCalledTimes(1);
		expect(sources).toHaveLength(1);
		expect(sources[0].connect).toHaveBeenCalledWith(mockCtx.destination);
		// First chunk starts at currentTime + FIRST_CHUNK_PRIMER_SECONDS (0.05)
		expect(sources[0].start).toHaveBeenCalledWith(1.05);
	});

	it("schedules the second chunk at end-of-first to keep playback gapless", async () => {
		mockCtx.currentTime = 0;
		// Buffer 0 lasts 0.004s (4 bytes * 0.001s), buffer 1 lasts 0.006s.
		const player = createSeamlessTtsPlayer();
		await player.append(new Uint8Array([1, 2, 3, 4]));
		await player.append(new Uint8Array([5, 6, 7, 8, 9, 10]));

		expect(sources).toHaveLength(2);
		expect(sources[0].start).toHaveBeenCalledWith(0.05);
		// Second source starts at 0.05 (primed start) + 0.004 (first buffer dur).
		// Use closeTo to tolerate float arithmetic in nextStartTime accumulation.
		const secondStart = sources[1].start.mock.calls[0][0] as number;
		expect(secondStart).toBeCloseTo(0.054, 6);
	});

	it("does not schedule new chunks after stop()", async () => {
		const player = createSeamlessTtsPlayer();
		await player.append(new Uint8Array([1, 2, 3, 4]));
		player.stop();
		expect(sources[0].stop).toHaveBeenCalled();

		await player.append(new Uint8Array([5, 6, 7, 8]));
		// Still just the one source from the pre-stop append.
		expect(sources).toHaveLength(1);
	});

	it("done() resolves when the last source ends", async () => {
		const player = createSeamlessTtsPlayer();
		await player.append(new Uint8Array([1, 2, 3, 4]));
		await player.append(new Uint8Array([5, 6, 7, 8]));

		const tail = sources[sources.length - 1];
		const donePromise = player.done();
		// Simulate the engine firing the end-of-source event.
		tail.onended?.();
		await expect(donePromise).resolves.toBeUndefined();
	});

	it("done() resolves immediately when nothing has been appended", async () => {
		const player = createSeamlessTtsPlayer();
		await expect(player.done()).resolves.toBeUndefined();
	});

	it("done() resolves immediately after stop()", async () => {
		const player = createSeamlessTtsPlayer();
		await player.append(new Uint8Array([1, 2, 3, 4]));
		player.stop();
		await expect(player.done()).resolves.toBeUndefined();
	});

	it("cleanup() closes the AudioContext and ignores duplicate calls", async () => {
		const player = createSeamlessTtsPlayer();
		await player.cleanup();
		expect(mockCtx.close).toHaveBeenCalledTimes(1);
		await player.cleanup();
		expect(mockCtx.close).toHaveBeenCalledTimes(1);
	});
});

describe("splitTextForTts", () => {
	it("returns empty array for empty string", () => {
		expect(splitTextForTts("")).toEqual([]);
	});

	it("returns empty array for whitespace-only string", () => {
		expect(splitTextForTts("   ")).toEqual([]);
	});

	it("returns single chunk for short text", () => {
		expect(splitTextForTts("Hello, world!")).toEqual(["Hello, world!"]);
	});

	it("splits on Japanese period (。)", () => {
		const sentence1 = `${"あ".repeat(20000)}。`;
		const sentence2 = `${"い".repeat(20000)}。`;
		const result = splitTextForTts(`${sentence1}${sentence2}`);
		expect(result.length).toBe(2);
		expect(result[0]).toBe(sentence1);
		expect(result[1]).toBe(sentence2);
	});

	it("splits on newlines", () => {
		const line1 = "a".repeat(20000);
		const line2 = "b".repeat(20000);
		const result = splitTextForTts(`${line1}\n${line2}`);
		expect(result.length).toBe(2);
		expect(result[0]).toBe(line1);
		expect(result[1]).toBe(line2);
	});

	it("hard-cuts when no sentence boundary found within limit", () => {
		const result = splitTextForTts("あ".repeat(35000));
		expect(result.length).toBe(2);
		expect(result[0].length).toBe(30000);
		expect(result[1].length).toBe(5000);
	});

	it("does not produce empty chunks", () => {
		const result = splitTextForTts("テスト。\n\nテスト2。");
		for (const chunk of result) {
			expect(chunk.length).toBeGreaterThan(0);
		}
	});

	it("handles text exactly at limit", () => {
		const text = "a".repeat(30000);
		expect(splitTextForTts(text)).toEqual([text]);
	});

	it("splits text exceeding 30000 chars into multiple chunks", () => {
		const part1 = `${"あ".repeat(25000)}。`;
		const part2 = "い".repeat(20000);
		const result = splitTextForTts(`${part1}${part2}`);
		expect(result.length).toBe(2);
		expect(result[0]).toBe(part1);
		expect(result[1]).toBe(part2);
	});
});
