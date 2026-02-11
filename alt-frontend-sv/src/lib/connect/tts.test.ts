import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock proto imports before importing module
vi.mock("$lib/gen/alt/tts/v1/tts_pb", () => ({
	TTSService: {
		typeName: "alt.tts.v1.TTSService",
	},
}));

vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
}));

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import {
	createTtsClient,
	synthesizeSpeech,
	synthesizeSpeechStream,
	listVoices,
	type SynthesizeResult,
} from "./tts";

describe("createTtsClient", () => {
	const mockTransport = {} as Transport;

	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("creates client with TTSService and transport", () => {
		createTtsClient(mockTransport);
		expect(createClient).toHaveBeenCalledWith(
			expect.objectContaining({ typeName: "alt.tts.v1.TTSService" }),
			mockTransport,
		);
	});
});

describe("synthesizeSpeech", () => {
	const mockTransport = {} as Transport;
	const mockSynthesize = vi.fn();

	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(createClient).mockReturnValue({
			synthesize: mockSynthesize,
			synthesizeStream: vi.fn(),
			listVoices: vi.fn(),
		} as unknown as ReturnType<typeof createClient>);
	});

	it("returns synthesized audio with default voice and speed", async () => {
		const audioData = new Uint8Array([1, 2, 3, 4]);
		mockSynthesize.mockResolvedValue({
			audioWav: audioData,
			sampleRate: 24000,
			durationSeconds: 1.5,
		});

		const result = await synthesizeSpeech(mockTransport, { text: "Hello" });

		expect(mockSynthesize).toHaveBeenCalledWith({
			text: "Hello",
			voice: "jf_gongitsune",
			speed: 1.25,
		});
		expect(result).toEqual({
			audioWav: audioData,
			sampleRate: 24000,
			durationSeconds: 1.5,
		});
	});

	it("uses custom voice and speed when provided", async () => {
		mockSynthesize.mockResolvedValue({
			audioWav: new Uint8Array([1]),
			sampleRate: 24000,
			durationSeconds: 0.5,
		});

		await synthesizeSpeech(mockTransport, {
			text: "Test",
			voice: "jm_beta",
			speed: 1.5,
		});

		expect(mockSynthesize).toHaveBeenCalledWith({
			text: "Test",
			voice: "jm_beta",
			speed: 1.5,
		});
	});

	it("propagates errors from the client", async () => {
		mockSynthesize.mockRejectedValue(new Error("TTS unavailable"));

		await expect(
			synthesizeSpeech(mockTransport, { text: "Fail" }),
		).rejects.toThrow("TTS unavailable");
	});
});

describe("synthesizeSpeechStream", () => {
	const mockTransport = {} as Transport;
	const mockSynthesizeStream = vi.fn();

	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(createClient).mockReturnValue({
			synthesize: vi.fn(),
			synthesizeStream: mockSynthesizeStream,
			listVoices: vi.fn(),
		} as unknown as ReturnType<typeof createClient>);
	});

	it("yields chunks from the streaming RPC with defaults", async () => {
		const chunk1 = {
			audioWav: new Uint8Array([1, 2]),
			sampleRate: 24000,
			durationSeconds: 0.5,
		};
		const chunk2 = {
			audioWav: new Uint8Array([3, 4]),
			sampleRate: 24000,
			durationSeconds: 0.7,
		};

		// Mock returns an async iterable
		mockSynthesizeStream.mockReturnValue(
			(async function* () {
				yield chunk1;
				yield chunk2;
			})(),
		);

		const results = [];
		for await (const result of synthesizeSpeechStream(mockTransport, {
			text: "Hello world",
		})) {
			results.push(result);
		}

		expect(mockSynthesizeStream).toHaveBeenCalledWith({
			text: "Hello world",
			voice: "jf_gongitsune",
			speed: 1.25,
		});
		expect(results).toHaveLength(2);
		expect(results[0]).toEqual(chunk1);
		expect(results[1]).toEqual(chunk2);
	});

	it("uses custom voice and speed when provided", async () => {
		mockSynthesizeStream.mockReturnValue(
			(async function* () {
				yield {
					audioWav: new Uint8Array([1]),
					sampleRate: 24000,
					durationSeconds: 0.3,
				};
			})(),
		);

		const results = [];
		for await (const result of synthesizeSpeechStream(mockTransport, {
			text: "Test",
			voice: "jm_beta",
			speed: 1.5,
		})) {
			results.push(result);
		}

		expect(mockSynthesizeStream).toHaveBeenCalledWith({
			text: "Test",
			voice: "jm_beta",
			speed: 1.5,
		});
	});

	it("propagates errors from the streaming client", async () => {
		const failingIterable: AsyncIterable<SynthesizeResult> = {
			[Symbol.asyncIterator]() {
				return {
					next: () => Promise.reject(new Error("Stream failed")),
				};
			},
		};
		mockSynthesizeStream.mockReturnValue(failingIterable);

		const results = [];
		await expect(async () => {
			for await (const result of synthesizeSpeechStream(mockTransport, {
				text: "Fail",
			})) {
				results.push(result);
			}
		}).rejects.toThrow("Stream failed");
	});
});

describe("listVoices", () => {
	const mockTransport = {} as Transport;
	const mockListVoices = vi.fn();

	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(createClient).mockReturnValue({
			synthesize: vi.fn(),
			synthesizeStream: vi.fn(),
			listVoices: mockListVoices,
		} as unknown as ReturnType<typeof createClient>);
	});

	it("returns available voices", async () => {
		mockListVoices.mockResolvedValue({
			voices: [
				{ id: "jf_alpha", name: "Alpha Female", gender: "female" },
				{ id: "jm_beta", name: "Beta Male", gender: "male" },
			],
		});

		const result = await listVoices(mockTransport);

		expect(result).toEqual([
			{ id: "jf_alpha", name: "Alpha Female", gender: "female" },
			{ id: "jm_beta", name: "Beta Male", gender: "male" },
		]);
	});

	it("returns empty array when no voices available", async () => {
		mockListVoices.mockResolvedValue({ voices: [] });

		const result = await listVoices(mockTransport);
		expect(result).toEqual([]);
	});
});
