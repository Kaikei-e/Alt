import type { Transport } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import { flushSync } from "svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
	SpeechChunk,
	SynthesizeSpeechStreamOptions,
} from "$lib/connect/tts";
import type { SeamlessAudioPlayer } from "$lib/utils/seamlessAudioPlayer";
import {
	createSummarySpeech,
	resetSummarySpeechSharedState,
} from "./useSummarySpeech.svelte";

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

		signal?.addEventListener("abort", () => {
			waitingResolver?.();
			waitingResolver = null;
		});

		control = {
			signal,
			async yieldChunk(chunk: SpeechChunk) {
				queue.push({ chunk });
				waitingResolver?.();
				waitingResolver = null;
				await new Promise((resolve) => setTimeout(resolve, 0));
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
		resetSummarySpeechSharedState();
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

		await vi.waitFor(() => expect(speech.state).toBe("playing"));
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

	it("stop() cancels in-flight stream, handles stream abort rejection, and resets state to idle (not error)", async () => {
		const { stream, getControls } = createControllableStream();

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: stream,
			});
		});

		const playPromise = speech.play("Utterance to stop");
		flushSync();

		const control = getControls();
		expect(speech.state).toBe("loading");

		speech.stop();
		flushSync();

		expect(control.signal?.aborted).toBe(true);
		expect(mockPlayerStop).toHaveBeenCalled();
		await playPromise;
		flushSync();

		expect(speech.state).toBe("idle");

		cleanup();
	});

	it("remains in playing state until waitUntilDrained completes", async () => {
		let resolveDrain!: () => void;
		const deferredDrain = new Promise<void>((resolve) => {
			resolveDrain = resolve;
		});
		mockPlayerWaitUntilDrained.mockImplementation(() => deferredDrain);

		const { stream, getControls } = createControllableStream();

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: stream,
			});
		});

		const playPromise = speech.play("Summary");
		flushSync();

		const control = getControls();
		await control.yieldChunk({
			audioWav: new Uint8Array([1]),
			sampleRate: 24000,
			durationSeconds: 1.0,
		});
		flushSync();

		control.finish();
		await vi.waitFor(() => expect(mockPlayerEnqueue).toHaveBeenCalled());

		// Stream has completed all chunks, but drain is still pending
		expect(speech.state).toBe("playing");

		// Resolve the drain
		resolveDrain();
		await playPromise;
		flushSync();

		expect(speech.state).toBe("idle");
		cleanup();
	});

	it("transitions to error when server returns Code.Canceled", async () => {
		const canceledErr = new ConnectError("Server canceled", Code.Canceled);

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				// biome-ignore lint/correctness/useYield: mock throwing immediately
				synthesizeStream: async function* () {
					throw canceledErr;
				},
			});
		});

		await speech.play("Text");
		flushSync();

		expect(speech.state).toBe("error");
		cleanup();
	});

	it("calls player.stop() on mid-stream error before transitioning to error", async () => {
		const { stream, getControls } = createControllableStream();

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: stream,
			});
		});

		const playPromise = speech.play("Summary text");
		flushSync();

		const control = getControls();
		await control.yieldChunk({
			audioWav: new Uint8Array([1, 2]),
			sampleRate: 24000,
			durationSeconds: 1.0,
		});
		flushSync();
		await vi.waitFor(() => expect(speech.state).toBe("playing"));

		// Mid-stream error
		control.error(new ConnectError("Network drop", Code.Unavailable));
		await playPromise;
		flushSync();

		expect(mockPlayerStop).toHaveBeenCalled();
		expect(speech.state).toBe("error");
		cleanup();
	});

	it("shares one AudioContext across multiple hook instances and does not recreate on repeated plays", async () => {
		const mockCreateAudioContext = vi.fn(() => mockAudioContext);

		let speech1!: ReturnType<typeof createSummarySpeech>;
		let speech2!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech1 = createSummarySpeech({
				createAudioContext: mockCreateAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: async function* () {},
			});
			speech2 = createSummarySpeech({
				createAudioContext: mockCreateAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: async function* () {},
			});
		});

		await speech1.play("First card");
		flushSync();
		expect(mockCreateAudioContext).toHaveBeenCalledTimes(1);

		await speech2.play("Second card");
		flushSync();
		expect(mockCreateAudioContext).toHaveBeenCalledTimes(1);

		await speech1.play("First card again");
		flushSync();
		expect(mockCreateAudioContext).toHaveBeenCalledTimes(1);

		cleanup();
	});

	it("creates a new AudioContext on second play if shared context is closed", async () => {
		const ctx1 = {
			state: "running" as AudioContextState,
			currentTime: 0,
			resume: vi.fn<() => Promise<void>>().mockResolvedValue(undefined),
		} as unknown as AudioContext;
		const ctx2 = {
			state: "running" as AudioContextState,
			currentTime: 0,
			resume: vi.fn<() => Promise<void>>().mockResolvedValue(undefined),
		} as unknown as AudioContext;

		let callCount = 0;
		const mockCreateAudioContext = vi.fn(() => {
			callCount++;
			return callCount === 1 ? ctx1 : ctx2;
		});

		let speech!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech = createSummarySpeech({
				createAudioContext: mockCreateAudioContext,
				createPlayer: () => mockPlayer,
				synthesizeStream: async function* () {},
			});
		});

		await speech.play("First card");
		flushSync();
		expect(mockCreateAudioContext).toHaveBeenCalledTimes(1);

		(ctx1 as { state: AudioContextState }).state = "closed";

		await speech.play("Second card");
		flushSync();
		expect(mockCreateAudioContext).toHaveBeenCalledTimes(2);

		cleanup();
	});

	it("module-level latch: a second hook instance initializes as unavailable if first got FailedPrecondition", async () => {
		const failedPreconditionErr = new ConnectError(
			"TTS is disabled",
			Code.FailedPrecondition,
		);

		let speech1!: ReturnType<typeof createSummarySpeech>;
		let speech2!: ReturnType<typeof createSummarySpeech>;
		const cleanup = $effect.root(() => {
			speech1 = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
				// biome-ignore lint/correctness/useYield: mock generator throwing immediately
				synthesizeStream: async function* () {
					throw failedPreconditionErr;
				},
			});
		});

		await speech1.play("Text");
		flushSync();
		expect(speech1.state).toBe("unavailable");

		// Create a second hook instance (like navigating to a new card)
		const cleanup2 = $effect.root(() => {
			speech2 = createSummarySpeech({
				audioContext: mockAudioContext,
				createPlayer: () => mockPlayer,
			});
		});

		expect(speech2.state).toBe("unavailable");
		mockResume.mockClear();
		await speech2.play("Text on new card");
		flushSync();
		expect(speech2.state).toBe("unavailable");
		expect(mockResume).not.toHaveBeenCalled();

		cleanup();
		cleanup2();
	});
});
