import { beforeEach, describe, expect, it, vi } from "vitest";
import { createSeamlessAudioPlayer } from "./seamlessAudioPlayer";

interface FakeSourceNode {
	buffer: AudioBuffer | null;
	startTime: number | null;
	stopped: boolean;
	onended: (() => void) | null;
	connect: ReturnType<typeof vi.fn>;
	start: ReturnType<typeof vi.fn>;
	stop: ReturnType<typeof vi.fn>;
}

class FakeAudioContext {
	currentTime = 1.0;
	destination = {};
	sources: FakeSourceNode[] = [];
	durations: number[] = [2.0, 1.5, 0.5];
	chunkIndex = 0;

	decodeAudioData = vi.fn(async (_buffer: ArrayBuffer) => {
		const duration = this.durations[this.chunkIndex++] ?? 1.0;
		return {
			duration,
			sampleRate: 24000,
			length: duration * 24000,
			numberOfChannels: 1,
		} as AudioBuffer;
	});

	createBufferSource = vi.fn(() => {
		const source: FakeSourceNode = {
			buffer: null,
			startTime: null,
			stopped: false,
			onended: null,
			connect: vi.fn(),
			start: vi.fn((time?: number) => {
				source.startTime = time ?? this.currentTime;
			}),
			stop: vi.fn(() => {
				source.stopped = true;
				if (source.onended) {
					source.onended();
				}
			}),
		};
		this.sources.push(source);
		return source as unknown as AudioBufferSourceNode;
	});
}

describe("createSeamlessAudioPlayer", () => {
	let fakeContext: FakeAudioContext;

	beforeEach(() => {
		vi.clearAllMocks();
		fakeContext = new FakeAudioContext();
	});

	it("schedules first chunk at currentTime + 0.05 and decodes with decodeAudioData", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);
		const dummyWav = new Uint8Array([1, 2, 3, 4]);

		await player.enqueue(dummyWav);

		expect(fakeContext.decodeAudioData).toHaveBeenCalledTimes(1);
		expect(fakeContext.createBufferSource).toHaveBeenCalledTimes(1);

		const source = fakeContext.sources[0];
		expect(source?.start).toHaveBeenCalledWith(1.05);
	});

	it("schedules subsequent chunks back-to-back based on previous buffer duration", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		// Chunk 0 duration = 2.0s -> scheduled at 1.05, ends at 3.05
		await player.enqueue(new Uint8Array([1]));
		// Chunk 1 duration = 1.5s -> scheduled at 3.05, ends at 4.55
		await player.enqueue(new Uint8Array([2]));
		// Chunk 2 duration = 0.5s -> scheduled at 4.55, ends at 5.05
		await player.enqueue(new Uint8Array([3]));

		expect(fakeContext.sources).toHaveLength(3);
		expect(fakeContext.sources[0]?.start).toHaveBeenCalledWith(1.05);
		expect(fakeContext.sources[1]?.start).toHaveBeenCalledWith(3.05);
		expect(fakeContext.sources[2]?.start).toHaveBeenCalledWith(4.55);
	});

	it("stop() stops all scheduled sources and prevents further scheduling", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		await player.enqueue(new Uint8Array([1]));
		await player.enqueue(new Uint8Array([2]));

		player.stop();

		for (const source of fakeContext.sources) {
			expect(source.stop).toHaveBeenCalled();
		}

		// Enqueuing after stop should not schedule new sources
		await player.enqueue(new Uint8Array([3]));
		expect(fakeContext.sources).toHaveLength(2);
	});

	it("waitUntilDrained() resolves when all enqueued audio sources have finished playing", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		await player.enqueue(new Uint8Array([1]));
		await player.enqueue(new Uint8Array([2]));

		let drained = false;
		const drainPromise = player.waitUntilDrained().then(() => {
			drained = true;
		});

		expect(drained).toBe(false);

		// End source 0
		fakeContext.sources[0]?.onended?.();
		await new Promise((r) => setTimeout(r, 0));
		expect(drained).toBe(false);

		// End source 1 (last source)
		fakeContext.sources[1]?.onended?.();
		await drainPromise;
		expect(drained).toBe(true);
	});

	it("waitUntilDrained() resolves immediately if no audio was enqueued", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		let drained = false;
		await player.waitUntilDrained().then(() => {
			drained = true;
		});

		expect(drained).toBe(true);
	});

	it("waitUntilDrained() resolves promptly when stop() is called", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		await player.enqueue(new Uint8Array([1]));

		let drained = false;
		const drainPromise = player.waitUntilDrained().then(() => {
			drained = true;
		});

		expect(drained).toBe(false);

		player.stop();
		await drainPromise;
		expect(drained).toBe(true);
	});

	it("schedules a late chunk at context.currentTime when currentTime > nextStartTime", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		// First chunk at 1.0 + 0.05 = 1.05. Duration is 2.0s -> nextStartTime is 3.05
		await player.enqueue(new Uint8Array([1]));
		expect(fakeContext.sources[0]?.startTime).toBe(1.05);

		// Late arrival: currentTime is now 5.0 (> 3.05)
		fakeContext.currentTime = 5.0;
		await player.enqueue(new Uint8Array([2]));

		// Must be scheduled at currentTime (5.0), not the past nextStartTime (3.05)
		expect(fakeContext.sources[1]?.startTime).toBe(5.0);
	});

	it("does not schedule a source if decode was pending when stop() ran", async () => {
		let resolveDecode!: (buffer: AudioBuffer) => void;
		fakeContext.decodeAudioData = vi.fn(
			() =>
				new Promise<AudioBuffer>((resolve) => {
					resolveDecode = resolve;
				}),
		);

		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		const enqueuePromise = player.enqueue(new Uint8Array([1, 2, 3]));

		// Stop while decodeAudioData is still in flight
		player.stop();

		// Now let decodeAudioData resolve
		resolveDecode({
			duration: 1.0,
			sampleRate: 24000,
			length: 24000,
			numberOfChannels: 1,
		} as AudioBuffer);

		await enqueuePromise;

		// No source should have been created or started
		expect(fakeContext.createBufferSource).not.toHaveBeenCalled();
	});

	it("decodes on its own copy when given a subarray with a byteOffset", async () => {
		const player = createSeamlessAudioPlayer(
			fakeContext as unknown as AudioContext,
		);

		const underlying = new Uint8Array([0, 0, 0, 0, 42, 43, 44, 45, 0, 0]);
		const subarray = underlying.subarray(4, 8); // offset 4, length 4

		await player.enqueue(subarray);

		expect(fakeContext.decodeAudioData).toHaveBeenCalledTimes(1);
		const decodedBuffer = fakeContext.decodeAudioData.mock
			.calls[0]?.[0] as ArrayBuffer;
		expect(decodedBuffer.byteLength).toBe(4);
		const decodedBytes = new Uint8Array(decodedBuffer);
		expect(Array.from(decodedBytes)).toEqual([42, 43, 44, 45]);

		// Mutating the original underlying buffer after enqueue must not affect the decoded copy
		underlying[4] = 99;
		expect(decodedBytes[0]).toBe(42);
	});
});
