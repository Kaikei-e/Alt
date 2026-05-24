import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	synthesizeSpeechStream: vi.fn(),
}));

vi.mock("$lib/utils/audio", () => ({
	createSeamlessTtsPlayer: vi.fn(),
	splitTextForTts: vi.fn((text: string) => [text]),
}));

import { synthesizeSpeechStream } from "$lib/connect";
import { createSeamlessTtsPlayer, splitTextForTts } from "$lib/utils/audio";
import { useTtsPlayback } from "./useTtsPlayback.svelte";

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

function makeMockPlayer() {
	const player = {
		append: vi.fn(async (_bytes: Uint8Array): Promise<void> => {}),
		stop: vi.fn(),
		done: vi.fn(async (): Promise<void> => {}),
		cleanup: vi.fn(async (): Promise<void> => {}),
	};
	return player;
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
		it("appends every streamed chunk into a single seamless player", async () => {
			const player = makeMockPlayer();
			vi.mocked(createSeamlessTtsPlayer).mockReturnValue(player);
			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1, 2, 3]),
						sampleRate: 44100,
						durationSeconds: 1.0,
					},
					{
						audioWav: new Uint8Array([4, 5, 6]),
						sampleRate: 44100,
						durationSeconds: 1.0,
					},
				])(),
			);

			const tts = useTtsPlayback();
			await tts.play("Hello");

			expect(createSeamlessTtsPlayer).toHaveBeenCalledTimes(1);
			expect(player.append).toHaveBeenCalledTimes(2);
			expect(player.append).toHaveBeenNthCalledWith(
				1,
				new Uint8Array([1, 2, 3]),
			);
			expect(player.append).toHaveBeenNthCalledWith(
				2,
				new Uint8Array([4, 5, 6]),
			);
			expect(player.done).toHaveBeenCalledTimes(1);
			expect(player.cleanup).toHaveBeenCalledTimes(1);
		});

		it("handles empty text gracefully", async () => {
			const tts = useTtsPlayback();
			await tts.play("");

			expect(synthesizeSpeechStream).not.toHaveBeenCalled();
			expect(createSeamlessTtsPlayer).not.toHaveBeenCalled();
			expect(tts.state).toBe("idle");
		});

		it("sets error state on stream failure and cleans up the player", async () => {
			const player = makeMockPlayer();
			vi.mocked(createSeamlessTtsPlayer).mockReturnValue(player);
			const failingStream: AsyncIterable<{
				audioWav: Uint8Array;
				sampleRate: number;
				durationSeconds: number;
			}> = {
				[Symbol.asyncIterator]() {
					return {
						next: () => Promise.reject(new Error("TTS service unavailable")),
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
			expect(player.cleanup).toHaveBeenCalled();
		});

		it("passes voice and speed options", async () => {
			const player = makeMockPlayer();
			vi.mocked(createSeamlessTtsPlayer).mockReturnValue(player);
			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1]),
						sampleRate: 44100,
						durationSeconds: 0.5,
					},
				])(),
			);

			const tts = useTtsPlayback();
			await tts.play("Test", { voice: "sup-F4", speed: 1.5 });

			expect(synthesizeSpeechStream).toHaveBeenCalledWith(
				expect.anything(),
				expect.objectContaining({
					text: "Test",
					voice: "sup-F4",
					speed: 1.5,
				}),
			);
		});

		it("splits long text into multiple chunks and feeds each into the same player", async () => {
			const player = makeMockPlayer();
			vi.mocked(createSeamlessTtsPlayer).mockReturnValue(player);
			vi.mocked(splitTextForTts).mockReturnValue(["chunk1", "chunk2"]);
			vi.mocked(synthesizeSpeechStream)
				.mockReturnValueOnce(
					createMockStream([
						{
							audioWav: new Uint8Array([1]),
							sampleRate: 44100,
							durationSeconds: 1.0,
						},
					])(),
				)
				.mockReturnValueOnce(
					createMockStream([
						{
							audioWav: new Uint8Array([2]),
							sampleRate: 44100,
							durationSeconds: 1.0,
						},
					])(),
				);

			const tts = useTtsPlayback();
			await tts.play("chunk1chunk2");

			expect(synthesizeSpeechStream).toHaveBeenCalledTimes(2);
			expect(createSeamlessTtsPlayer).toHaveBeenCalledTimes(1);
			expect(player.append).toHaveBeenCalledTimes(2);
			expect(player.done).toHaveBeenCalledTimes(1);
		});
	});

	describe("stop()", () => {
		it("stops the player and resets state to idle", async () => {
			const player = makeMockPlayer();
			let releaseDone: (() => void) | undefined;
			player.done.mockReturnValue(
				new Promise<void>((resolve) => {
					releaseDone = resolve;
				}),
			);
			vi.mocked(createSeamlessTtsPlayer).mockReturnValue(player);
			vi.mocked(synthesizeSpeechStream).mockReturnValue(
				createMockStream([
					{
						audioWav: new Uint8Array([1]),
						sampleRate: 44100,
						durationSeconds: 1.0,
					},
				])(),
			);

			const tts = useTtsPlayback();
			const playPromise = tts.play("Hello World");
			await vi.waitFor(() => {
				expect(player.append).toHaveBeenCalled();
			});

			tts.stop();

			expect(player.stop).toHaveBeenCalled();
			expect(player.cleanup).toHaveBeenCalled();
			(releaseDone as (() => void) | undefined)?.();
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
