import type { Transport } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { synthesizeSpeechStream } from "./tts";

const mockSynthesizeStream = vi.fn();

vi.mock("@connectrpc/connect", () => ({
	createClient: () => ({
		synthesizeStream: mockSynthesizeStream,
	}),
}));

describe("synthesizeSpeechStream", () => {
	const mockTransport = {} as Transport;

	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("calls client.synthesizeStream with text, speed, and abort signal", async () => {
		const controller = new AbortController();
		mockSynthesizeStream.mockImplementation(async function* () {
			// empty stream
		});

		const stream = synthesizeSpeechStream(
			mockTransport,
			{ text: "要約のテキストです", speed: 1.25 },
			controller.signal,
		);

		for await (const _chunk of stream) {
			// consume
		}

		expect(mockSynthesizeStream).toHaveBeenCalledWith(
			{
				text: "要約のテキストです",
				speed: 1.25,
			},
			{
				signal: controller.signal,
			},
		);
	});

	it("maps stream responses to SpeechChunk objects", async () => {
		const chunk1Wav = new Uint8Array([1, 2, 3]);
		const chunk2Wav = new Uint8Array([4, 5, 6]);

		mockSynthesizeStream.mockImplementation(async function* () {
			yield {
				audioWav: chunk1Wav,
				sampleRate: 24000,
				durationSeconds: 1.5,
			};
			yield {
				audioWav: chunk2Wav,
				sampleRate: 24000,
				durationSeconds: 2.0,
			};
		});

		const results = [];
		for await (const chunk of synthesizeSpeechStream(mockTransport, {
			text: "テスト",
		})) {
			results.push(chunk);
		}

		expect(results).toEqual([
			{
				audioWav: chunk1Wav,
				sampleRate: 24000,
				durationSeconds: 1.5,
			},
			{
				audioWav: chunk2Wav,
				sampleRate: 24000,
				durationSeconds: 2.0,
			},
		]);
	});

	it("propagates errors thrown by the underlying stream", async () => {
		const streamError = new Error("stream connection failed");
		mockSynthesizeStream.mockImplementation(async function* () {
			yield {
				audioWav: new Uint8Array([1]),
				sampleRate: 24000,
				durationSeconds: 0.5,
			};
			throw streamError;
		});

		const stream = synthesizeSpeechStream(mockTransport, {
			text: "エラーテスト",
		});

		await expect(async () => {
			for await (const _chunk of stream) {
				// consume
			}
		}).rejects.toThrow("stream connection failed");
	});
});
