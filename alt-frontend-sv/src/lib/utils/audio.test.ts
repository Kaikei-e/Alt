import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createAudioFromWav, splitTextForTts } from "./audio";

describe("createAudioFromWav", () => {
	let mockAudio: {
		play: ReturnType<typeof vi.fn>;
		pause: ReturnType<typeof vi.fn>;
		src: string;
		currentTime: number;
		addEventListener: ReturnType<typeof vi.fn>;
		removeEventListener: ReturnType<typeof vi.fn>;
	};
	let mockUrl: string;
	const originalURL = globalThis.URL;
	const OriginalAudio = globalThis.Audio;

	beforeEach(() => {
		mockUrl = "blob:http://localhost/fake-url";
		mockAudio = {
			play: vi.fn().mockResolvedValue(undefined),
			pause: vi.fn(),
			src: "",
			currentTime: 0,
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
		};

		// Mock URL.createObjectURL / revokeObjectURL
		globalThis.URL = Object.assign(
			((...args: ConstructorParameters<typeof URL>) =>
				new originalURL(...args)) as unknown as typeof URL,
			{
				...originalURL,
				createObjectURL: vi.fn().mockReturnValue(mockUrl),
				revokeObjectURL: vi.fn(),
			},
		);

		// Mock Audio constructor using Proxy to intercept `new` calls
		globalThis.Audio = new Proxy(class {} as typeof Audio, {
			construct: () => mockAudio as unknown as object,
		});
	});

	afterEach(() => {
		globalThis.URL = originalURL;
		globalThis.Audio = OriginalAudio;
	});

	it("creates an AudioPlayer from WAV bytes", () => {
		const wavBytes = new Uint8Array([82, 73, 70, 70]); // "RIFF"
		const player = createAudioFromWav(wavBytes);

		expect(URL.createObjectURL).toHaveBeenCalled();
		expect(player).toBeDefined();
		expect(player.play).toBeInstanceOf(Function);
		expect(player.stop).toBeInstanceOf(Function);
		expect(player.cleanup).toBeInstanceOf(Function);
		expect(player.onEnded).toBeInstanceOf(Function);
	});

	it("throws on empty data", () => {
		expect(() => createAudioFromWav(new Uint8Array(0))).toThrow(
			"Empty audio data",
		);
	});

	it("play() calls audio.play()", async () => {
		const player = createAudioFromWav(new Uint8Array([1, 2, 3]));
		await player.play();
		expect(mockAudio.play).toHaveBeenCalled();
	});

	it("stop() pauses and resets audio", () => {
		const player = createAudioFromWav(new Uint8Array([1, 2, 3]));
		player.stop();
		expect(mockAudio.pause).toHaveBeenCalled();
		expect(mockAudio.currentTime).toBe(0);
	});

	it("cleanup() revokes object URL", () => {
		const player = createAudioFromWav(new Uint8Array([1, 2, 3]));
		player.cleanup();
		expect(URL.revokeObjectURL).toHaveBeenCalledWith(mockUrl);
	});

	it("onEnded() sets ended callback", () => {
		const player = createAudioFromWav(new Uint8Array([1, 2, 3]));
		const cb = vi.fn();
		player.onEnded(cb);
		expect(mockAudio.addEventListener).toHaveBeenCalledTimes(1);
		const [event, handler, options] = mockAudio.addEventListener.mock.calls[0];
		expect(event).toBe("ended");
		expect(handler).toBe(cb);
		expect(options.once).toBe(true);
	});

	it("cleanup after stop does not throw", () => {
		const player = createAudioFromWav(new Uint8Array([1, 2, 3]));
		player.stop();
		expect(() => player.cleanup()).not.toThrow();
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
		const text = "Hello, world!";
		const result = splitTextForTts(text);
		expect(result).toEqual(["Hello, world!"]);
	});

	it("splits on Japanese period (。)", () => {
		const sentence1 = `${"あ".repeat(3000)}。`;
		const sentence2 = `${"い".repeat(3000)}。`;
		const text = `${sentence1}${sentence2}`;
		const result = splitTextForTts(text);
		expect(result.length).toBe(2);
		expect(result[0]).toBe(sentence1);
		expect(result[1]).toBe(sentence2);
	});

	it("splits on newlines", () => {
		const line1 = "a".repeat(3000);
		const line2 = "b".repeat(3000);
		const text = `${line1}\n${line2}`;
		const result = splitTextForTts(text);
		expect(result.length).toBe(2);
		expect(result[0]).toBe(line1);
		expect(result[1]).toBe(line2);
	});

	it("hard-cuts when no sentence boundary found within limit", () => {
		const text = "あ".repeat(6000);
		const result = splitTextForTts(text);
		expect(result.length).toBe(2);
		expect(result[0].length).toBe(5000);
		expect(result[1].length).toBe(1000);
	});

	it("does not produce empty chunks", () => {
		const text = "テスト。\n\nテスト2。";
		const result = splitTextForTts(text);
		for (const chunk of result) {
			expect(chunk.length).toBeGreaterThan(0);
		}
	});

	it("handles text exactly at limit", () => {
		const text = "a".repeat(5000);
		const result = splitTextForTts(text);
		expect(result).toEqual([text]);
	});
});
