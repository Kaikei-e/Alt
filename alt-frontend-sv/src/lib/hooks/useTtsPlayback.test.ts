import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	synthesizeSpeech: vi.fn(),
}));

vi.mock("$lib/utils/audio", () => ({
	splitTextForTts: vi.fn(),
	createAudioFromWav: vi.fn(),
}));

import { synthesizeSpeech } from "$lib/connect";
import { splitTextForTts, createAudioFromWav } from "$lib/utils/audio";
import { useTtsPlayback } from "./useTtsPlayback.svelte";

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

			vi.mocked(splitTextForTts).mockReturnValue(["Hello"]);
			vi.mocked(synthesizeSpeech).mockResolvedValue({
				audioWav: new Uint8Array([1, 2, 3]),
				sampleRate: 24000,
				durationSeconds: 1.0,
			});
			vi.mocked(createAudioFromWav).mockReturnValue(mockPlayer);

			const tts = useTtsPlayback();
			await tts.play("Hello");

			expect(splitTextForTts).toHaveBeenCalledWith("Hello");
			expect(synthesizeSpeech).toHaveBeenCalled();
			expect(createAudioFromWav).toHaveBeenCalledWith(
				new Uint8Array([1, 2, 3]),
			);
			expect(mockPlayer.play).toHaveBeenCalled();
		});

		it("handles empty text gracefully", async () => {
			vi.mocked(splitTextForTts).mockReturnValue([]);

			const tts = useTtsPlayback();
			await tts.play("");

			expect(synthesizeSpeech).not.toHaveBeenCalled();
			expect(tts.state).toBe("idle");
		});

		it("sets error state on synthesis failure", async () => {
			vi.mocked(splitTextForTts).mockReturnValue(["Hello"]);
			vi.mocked(synthesizeSpeech).mockRejectedValue(
				new Error("TTS service unavailable"),
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

			vi.mocked(splitTextForTts).mockReturnValue(["Test"]);
			vi.mocked(synthesizeSpeech).mockResolvedValue({
				audioWav: new Uint8Array([1]),
				sampleRate: 24000,
				durationSeconds: 0.5,
			});
			vi.mocked(createAudioFromWav).mockReturnValue(mockPlayer);

			const tts = useTtsPlayback();
			await tts.play("Test", { voice: "jm_beta", speed: 1.5 });

			expect(synthesizeSpeech).toHaveBeenCalledWith(
				expect.anything(),
				expect.objectContaining({
					text: "Test",
					voice: "jm_beta",
					speed: 1.5,
				}),
			);
		});

		it("processes multiple chunks sequentially", async () => {
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

			vi.mocked(splitTextForTts).mockReturnValue(["Chunk 1", "Chunk 2"]);
			vi.mocked(synthesizeSpeech)
				.mockResolvedValueOnce({
					audioWav: new Uint8Array([1]),
					sampleRate: 24000,
					durationSeconds: 1.0,
				})
				.mockResolvedValueOnce({
					audioWav: new Uint8Array([2]),
					sampleRate: 24000,
					durationSeconds: 1.0,
				});
			vi.mocked(createAudioFromWav)
				.mockReturnValueOnce(players[0])
				.mockReturnValueOnce(players[1]);

			const tts = useTtsPlayback();
			await tts.play("Chunk 1 Chunk 2");

			expect(synthesizeSpeech).toHaveBeenCalledTimes(2);
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

			vi.mocked(splitTextForTts).mockReturnValue(["Hello", "World"]);
			vi.mocked(synthesizeSpeech).mockResolvedValue({
				audioWav: new Uint8Array([1]),
				sampleRate: 24000,
				durationSeconds: 1.0,
			});
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
