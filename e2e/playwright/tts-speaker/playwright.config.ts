import { defineApiSuite } from "../_shared/config.js";

/**
 * tts-speaker Playwright API suite.
 *
 * Exercises Connect server-streaming contract for alt.tts.v1.TTSService/SynthesizeStream
 * and health endpoints.
 */
export default defineApiSuite({
	service: "tts-speaker",
	workers: 1,
	timeout: 30_000,
});
