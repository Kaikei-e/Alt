import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	synthesizeSpeechStream: vi.fn(),
}));

vi.mock("$lib/utils/audio", () => ({
	createAudioFromWav: vi.fn(),
}));

import { synthesizeSpeechStream } from "$lib/connect";
import { createAudioFromWav } from "$lib/utils/audio";
import { useTtsPlayback } from "./useTtsPlayback.svelte";

/** Helper: creates an async generator that yields the given chunks */
function createMockStream(
	chunks: Array<{
		audioWav: Uint8Array;
		sampleRate: number;
		durationSeconds: number;
	}>,
) {
	return async function* () {
		for (const chunk of chunks) {
			yield chunk;
		}
	};
}

describe("useTtsPlayback", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("initial state", () => {
		it("starts in idle state", () => {
			const tts = useTtsPlayback();
			expect(tts.state).toBe("idle");
			expect(tts.isPlaying).toBe(false);
			expect(tts.isLoading).toBe(false);
			expect(tts.error).toBeNull();
		});
	});

	describe("play()", () => {
		it("transitions through loading to playing state", async () => {
			const mockPlayer = {
				play: vi.fn().mockResolvedValue(undefined),
				stop: vi.fn(),
				cleanup: vi.fn(),
				onEnded: vi.fn().mockImplementation((cb) => cb()),
			};

			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1, 2, 3]),
						sampleRate: 24000,
						durationSeconds: 1.0,
					},
				])(),
			);
			vi.mocked(createAudioFromWav).mockReturnValue(mockPlayer);

			const tts = useTtsPlayback();
			await tts.play("Hello");

			expect(synthesizeSpeechStream).toHaveBeenCalled();
			expect(createAudioFromWav).toHaveBeenCalledWith(
				new Uint8Array([1, 2, 3]),
			);
			expect(mockPlayer.play).toHaveBeenCalled();
		});

		it("handles empty text gracefully", async () => {
			const tts = useTtsPlayback();
			await tts.play("");

			expect(synthesizeSpeechStream).not.toHaveBeenCalled();
			expect(tts.state).toBe("idle");
		});

		it("sets error state on stream failure", async () => {
			const failingStream: AsyncIterable<{
				audioWav: Uint8Array;
				sampleRate: number;
				durationSeconds: number;
			}> = {
				[Symbol.asyncIterator]() {
					return {
						next: () =>
							Promise.reject(new Error("TTS service unavailable")),
					};
				},
			};
			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				failingStream as AsyncGenerator<{
					audioWav: Uint8Array;
					sampleRate: number;
					durationSeconds: number;
				}>,
			);

			const tts = useTtsPlayback();
			await tts.play("Hello");

			expect(tts.state).toBe("error");
			expect(tts.error).toBe("TTS service unavailable");
		});

		it("passes voice and speed options", async () => {
			const mockPlayer = {
				play: vi.fn().mockResolvedValue(undefined),
				stop: vi.fn(),
				cleanup: vi.fn(),
				onEnded: vi.fn().mockImplementation((cb) => cb()),
			};

			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1]),
						sampleRate: 24000,
						durationSeconds: 0.5,
					},
				])(),
			);
			vi.mocked(createAudioFromWav).mockReturnValue(mockPlayer);

			const tts = useTtsPlayback();
			await tts.play("Test", { voice: "jm_beta", speed: 1.5 });

			expect(synthesizeSpeechStream).toHaveBeenCalledWith(
				expect.anything(),
				expect.objectContaining({
					text: "Test",
					voice: "jm_beta",
					speed: 1.5,
				}),
			);
		});

		it("processes multiple streamed chunks sequentially", async () => {
			const players = [
				{
					play: vi.fn().mockResolvedValue(undefined),
					stop: vi.fn(),
					cleanup: vi.fn(),
					onEnded: vi.fn().mockImplementation((cb) => cb()),
				},
				{
					play: vi.fn().mockResolvedValue(undefined),
					stop: vi.fn(),
					cleanup: vi.fn(),
					onEnded: vi.fn().mockImplementation((cb) => cb()),
				},
			];

			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1]),
						sampleRate: 24000,
						durationSeconds: 1.0,
					},
					{
						audioWav: new Uint8Array([2]),
						sampleRate: 24000,
						durationSeconds: 1.0,
					},
				])(),
			);
			vi.mocked(createAudioFromWav)
				.mockReturnValueOnce(players[0])
				.mockReturnValueOnce(players[1]);

			const tts = useTtsPlayback();
			await tts.play("Multiple sentences.");

			expect(players[0].play).toHaveBeenCalled();
			expect(players[1].play).toHaveBeenCalled();
			expect(players[0].cleanup).toHaveBeenCalled();
		});
	});

	describe("stop()", () => {
		it("stops and cleans up current player", async () => {
			const mockPlayer = {
				play: vi.fn().mockResolvedValue(undefined),
				stop: vi.fn(),
				cleanup: vi.fn(),
				onEnded: vi.fn(), // Don't call cb - keep playing
			};

			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1]),
						sampleRate: 24000,
						durationSeconds: 1.0,
					},
					{
						audioWav: new Uint8Array([2]),
						sampleRate: 24000,
						durationSeconds: 1.0,
					},
				])(),
			);
			vi.mocked(createAudioFromWav).mockReturnValue(mockPlayer);

			const tts = useTtsPlayback();
			// Start playing but don't await (it's waiting for onEnded)
			const playPromise = tts.play("Hello World");
			// Wait for first chunk to start
			await vi.waitFor(() => {
				expect(mockPlayer.play).toHaveBeenCalled();
			});

			tts.stop();

			expect(mockPlayer.stop).toHaveBeenCalled();
			expect(mockPlayer.cleanup).toHaveBeenCalled();

			// Let the play promise settle
			await playPromise;
			expect(tts.state).toBe("idle");
		});

		it("does nothing when idle", () => {
			const tts = useTtsPlayback();
			expect(() => tts.stop()).not.toThrow();
			expect(tts.state).toBe("idle");
		});
	});
});
