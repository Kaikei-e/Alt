import { decodeConnectFrames, encodeConnectFrame } from "../src/connect_stream.js";
import { expect, test } from "../src/fixtures.js";

/**
 * Contract tests for alt.tts.v1.TTSService/SynthesizeStream over Connect server streaming.
 *
 * Enforces the envelope protocol:
 * - Request is length-prefixed application/connect+json
 * - Response is HTTP 200 with application/connect+json
 * - Data frames decode to base64 audioWav (valid RIFF/WAVE), integer sampleRate, number durationSeconds
 * - Successful stream terminates with an end-of-stream frame without error
 * - Empty text terminates with an end-of-stream frame carrying invalid_argument
 */

test.describe("alt.tts.v1.TTSService/SynthesizeStream @contract", () => {
	test("SynthesizeStream yields audio frames with valid RIFF header and clean end-stream @contract", async ({
		tts,
	}) => {
		// Input > 60 chars to force multiple chunks (default max_chunk_chars: 60)
		const text =
			"これは音声合成ストリーミングのテストです。長いテキストを送信して複数のチャンクに分割されることを確認します。最後の文です。";
		const requestData = encodeConnectFrame({ text });
		const response = await tts.post("/alt.tts.v1.TTSService/SynthesizeStream", {
			headers: {
				"Content-Type": "application/connect+json",
				"Connect-Protocol-Version": "1",
			},
			data: requestData,
		});

		expect(response.status()).toBe(200);
		expect(response.headers()["content-type"] ?? "").toContain("application/connect+json");

		const body = await response.body();
		const frames = decodeConnectFrames(body);

		const dataFrames = frames.filter((f) => f.flag === 0);
		const endFrames = frames.filter((f) => f.flag === 2);

		expect(dataFrames.length).toBeGreaterThanOrEqual(2);
		for (const frame of dataFrames) {
			expect(typeof frame.data.audioWav).toBe("string");
			const wavBytes = Buffer.from(frame.data.audioWav, "base64");
			expect(wavBytes.length).toBeGreaterThan(44);
			expect(wavBytes.subarray(0, 4).toString("ascii")).toBe("RIFF");
			expect(wavBytes.subarray(8, 12).toString("ascii")).toBe("WAVE");

			expect(Number.isInteger(frame.data.sampleRate)).toBe(true);
			expect(frame.data.sampleRate).toBe(48000);

			expect(typeof frame.data.durationSeconds).toBe("number");
			expect(frame.data.durationSeconds).toBeGreaterThan(0);
		}

		// The stub produces 0.1s chunks; default chunk_gap_ms is 200ms (0.2s).
		// Non-last chunks have trailing silence gap (0.1 + 0.2 = 0.3s); last chunk has no gap (0.1s).
		const chunkDuration = 0.1;
		const gapDuration = 0.2;
		for (let i = 0; i < dataFrames.length - 1; i++) {
			expect(dataFrames[i]?.data.durationSeconds).toBeCloseTo(chunkDuration + gapDuration, 2);
		}
		const lastFrame = dataFrames[dataFrames.length - 1];
		expect(lastFrame?.data.durationSeconds).toBeCloseTo(chunkDuration, 2);

		expect(endFrames.length).toBe(1);
		expect(endFrames[0]?.data.error).toBeUndefined();
	});

	test("SynthesizeStream with empty text yields invalid_argument error frame @contract", async ({
		tts,
	}) => {
		const requestData = encodeConnectFrame({ text: "" });
		const response = await tts.post("/alt.tts.v1.TTSService/SynthesizeStream", {
			headers: {
				"Content-Type": "application/connect+json",
				"Connect-Protocol-Version": "1",
			},
			data: requestData,
		});

		expect(response.status()).toBe(200);
		expect(response.headers()["content-type"] ?? "").toContain("application/connect+json");

		const body = await response.body();
		const frames = decodeConnectFrames(body);

		const endFrames = frames.filter((f) => f.flag === 2);
		expect(endFrames.length).toBe(1);
		expect(endFrames[0]?.data.error).toBeDefined();
		expect(endFrames[0]?.data.error?.code).toBe("invalid_argument");
	});
});
