import type { Transport } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { flushSync } from "svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
	SpeechChunk,
	SynthesizeSpeechStreamOptions,
} from "$lib/connect/tts";
import type { SeamlessAudioPlayer } from "$lib/utils/seamlessAudioPlayer";
import { createSummarySpeech } from "./useSummarySpeech.svelte";

interface MockStreamControl {
	yieldChunk(chunk: SpeechChunk): Promise<void>;
	finish(): void;
	error(err: Error): void;
	signal?: AbortSignal;
}

function createControllableStream(): {
	stream: (
		transport: Transport,
		options: SynthesizeSpeechStreamOptions,
		signal?: AbortSignal,
	) => AsyncIterable<SpeechChunk>;
	getControls(): MockStreamControl;
} {
	let control!: MockStreamControl;

	const stream = (
		_transport: Transport,
		_options: SynthesizeSpeechStreamOptions,
		signal?: AbortSignal,
	) => {
		const queue: Array<{
			chunk?: SpeechChunk;
			done?: boolean;
			error?: Error;
		}> = [];
		let waitingResolver: (() => void) | null = null;

		control = {
			signal,
			async yieldChunk(chunk: SpeechChunk) {
				queue.push({ chunk });
				waitingResolver?.();
				waitingResolver = null;
			},
			finish() {
				queue.push({ done: true });
				waitingResolver?.();
				waitingResolver = null;
			},
			error(err: Error) {
				queue.push({ error: err });
				waitingResolver?.();
				waitingResolver = null;
			},
		};

		return {
			[Symbol.asyncIterator]() {
				return {
					async next(): Promise<IteratorResult<SpeechChunk>> {
						while (queue.length === 0) {
							if (signal?.aborted) {
								throw new Error("Aborted");
							}
							await new Promise<void>((resolve) => {
								waitingResolver = resolve;
							});
						}

						const item = queue.shift();
						if (!item) {
							return { done: true, value: undefined };
						}
						if (item.error) {
							throw item.error;
						}
						if (item.done || !item.chunk) {
							return { done: true, value: undefined };
						}
						return { done: false, value: item.chunk };
					},
				};
			},
		};
	};

	return {
		stream,
		getControls: () => control,
	};
}

describe("useSummarySpeech", () => {
	let mockResume: ReturnType<typeof vi.fn<() => Promise<void>>>;
	let mockAudioContext: AudioContext;
	let mockPlayer: SeamlessAudioPlayer;
	let mockPlayerStop: ReturnType<typeof vi.fn<() => void>>;
	let mockPlayerEnqueue: ReturnType<
		typeof vi.fn<(wav: Uint8Array) => Promise<void>>
	>;
	let mockPlayerWaitUntilDrained: ReturnType<typeof vi.fn<() => Promise<void>>>;

	beforeEach(() => {
		vi.clearAllMocks();
		mockResume = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
		mockAudioContext = {
			state: "suspended",
			currentTime: 0,
			resume: mockResume,
		} as unknown as AudioContext;

		mockPlayerStop = vi.fn<() => void>();
		mockPlayerEnqueue = vi
			.fn<(wav: Uint8Array) => Promise<void>>()
			.mockResolvedValue(undefined);
		mockPlayerWaitUntilDrained = vi
			.fn<() => Promise<void>>()
			.mockResolvedValue(undefined);

		mockPlayer = {
			enqueue: (wav: Uint8Array) => mockPlayerEnqueue(wav),
			stop: () => mockPlayerStop(),
			waitUntilDrained: () => mockPlayerWaitUntilDrained(),
		};
	});

	it("initializes with idle state", () => {
		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
			});
		});

		expect(speech.state).toBe("idle");
		cleanup();
	});

	it("resumes AudioContext synchronously within play() as required by user gesture policy", () => {
		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
			});
		});

		speech.play("Test summary text");

		// Crucial assertion: AudioContext.resume MUST be called synchronously inside play()
		expect(mockResume).toHaveBeenCalledTimes(1);

		cleanup();
	});

	it("transitions through loading -> playing -> idle during normal playback", async () => {
		const { stream, getControls } = createControllableStream();

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: stream,
			});
		});

		const playPromise = speech.play("Summary to synthesize");
		flushSync();

		expect(speech.state).toBe("loading");

		const control = getControls();
		await control.yieldChunk({
			audioWav: new Uint8Array([1, 2, 3]),
			sampleRate: 24000,
			durationSeconds: 1.0,
		});
		flushSync();

		expect(speech.state).toBe("playing");
		expect(mockPlayerEnqueue).toHaveBeenCalled();

		control.finish();
		await playPromise;
		flushSync();

		expect(speech.state).toBe("idle");
		cleanup();
	});

	it("latches on FailedPrecondition: transitions to unavailable and remains latched on retry", async () => {
		const failedPreconditionErr = new ConnectError(
			"TTS is disabled",
			Code.FailedPrecondition,
		);

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				// biome-ignore lint/correctness/useYield: mock generator throwing immediately
				synthesizeStream: async function* () {
					throw failedPreconditionErr;
				},
			});
		});

		await speech.play("Text");
		flushSync();

		expect(speech.state).toBe("unavailable");

		// Latch verification: calling play again must remain unavailable and not retry
		mockResume.mockClear();
		await speech.play("Text again");
		flushSync();

		expect(speech.state).toBe("unavailable");
		expect(mockResume).not.toHaveBeenCalled();

		cleanup();
	});

	it("transitions to error on Unavailable and allows retry", async () => {
		let attempt = 0;
		const unavailableErr = new ConnectError(
			"TTS unavailable",
			Code.Unavailable,
		);

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: async function* () {
					attempt++;
					if (attempt === 1) {
						throw unavailableErr;
					}
					yield {
						audioWav: new Uint8Array([1]),
						sampleRate: 24000,
						durationSeconds: 0.5,
					};
				},
			});
		});

		// First attempt fails with Unavailable -> error state
		await speech.play("First attempt");
		flushSync();

		expect(speech.state).toBe("error");

		// Second attempt succeeds -> retry is permitted
		await speech.play("Second attempt");
		flushSync();

		expect(speech.state).toBe("idle");
		expect(attempt).toBe(2);

		cleanup();
	});

	it("cancels previous stream via AbortController when play is called twice", async () => {
		const { stream, getControls } = createControllableStream();

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: stream,
			});
		});

		void speech.play("First utterance");
		flushSync();

		const firstControl = getControls();
		expect(firstControl.signal?.aborted).toBe(false);

		// Second play call while first is in flight
		void speech.play("Second utterance");
		flushSync();

		// First signal must be aborted
		expect(firstControl.signal?.aborted).toBe(true);
		expect(mockPlayerStop).toHaveBeenCalled();

		cleanup();
	});

	it("stop() cancels in-flight stream, stops player, and resets state to idle", async () => {
		const { stream, getControls } = createControllableStream();

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: stream,
			});
		});

		void speech.play("Utterance to stop");
		flushSync();

		const control = getControls();
		expect(speech.state).toBe("loading");

		speech.stop();
		flushSync();

		expect(control.signal?.aborted).toBe(true);
		expect(mockPlayerStop).toHaveBeenCalled();
		expect(speech.state).toBe("idle");

		cleanup();
	});
});
