import { CONNECT_RPC_PATHS } from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import {
	fulfillConnectError,
	fulfillConnectStream,
	fulfillJson,
} from "../../utils/mockHelpers";

/**
 * Creates a minimal valid 16-bit mono PCM WAV buffer encoded as base64.
 */
function createTinyValidWavBase64(): string {
	const sampleRate = 24000;
	const numSamples = 240;
	const dataSize = numSamples * 2;
	const buffer = Buffer.alloc(44 + dataSize);

	buffer.write("RIFF", 0);
	buffer.writeUInt32LE(36 + dataSize, 4);
	buffer.write("WAVE", 8);
	buffer.write("fmt ", 12);
	buffer.writeUInt32LE(16, 16);
	buffer.writeUInt16LE(1, 20); // PCM
	buffer.writeUInt16LE(1, 22); // Mono
	buffer.writeUInt32LE(sampleRate, 24);
	buffer.writeUInt32LE(sampleRate * 2, 28); // ByteRate
	buffer.writeUInt16LE(2, 32); // BlockAlign
	buffer.writeUInt16LE(16, 34); // BitsPerSample
	buffer.write("data", 36);
	buffer.writeUInt32LE(dataSize, 40);

	return buffer.toString("base64");
}

const TINY_WAV_BASE64 = createTinyValidWavBase64();

/**
 * Three Connect-RPC SynthesizeStream response chunks.
 */
const TTS_THREE_CHUNKS = [
	{
		audioWav: TINY_WAV_BASE64,
		sampleRate: 24000,
		durationSeconds: 0.1,
	},
	{
		audioWav: TINY_WAV_BASE64,
		sampleRate: 24000,
		durationSeconds: 0.1,
	},
	{
		audioWav: TINY_WAV_BASE64,
		sampleRate: 24000,
		durationSeconds: 0.1,
	},
];

/**
 * Mock summary stream chunks.
 */
const SUMMARY_STREAM_CHUNKS = [
	{
		chunk: "This is the generated AI summary for visual preview speech test.",
		isFinal: true,
		articleId: "art-speech-1",
		isCached: false,
	},
];

/**
 * Fake AudioContext script recording decodeAudioData and start calls on window.
 */
const FAKE_AUDIO_INIT_SCRIPT = `
(function () {
  class FakeAudioBuffer {
    constructor() {
      this.duration = 600;
      this.sampleRate = 24000;
      this.numberOfChannels = 1;
    }
  }
  class FakeAudioBufferSourceNode {
    constructor() {
      this.buffer = null;
      this.onended = null;
      this._stopped = false;
    }
    connect() {}
    start(time) {
      window.__audioStartCalls = (window.__audioStartCalls || 0) + 1;
      window.__audioStartTimes = window.__audioStartTimes || [];
      window.__audioStartTimes.push(time);
    }
    stop() {
      if (this._stopped) return;
      this._stopped = true;
      setTimeout(() => {
        try {
          if (this.onended) this.onended();
        } catch (_) {}
      }, 0);
    }
  }
  class FakeAudioContext {
    constructor() {
      this.state = "running";
      this.currentTime = 0;
      this.destination = {};
      window.__audioContextResumed = false;
    }
    resume() {
      window.__audioContextResumed = true;
      this.state = "running";
      return Promise.resolve();
    }
    createBufferSource() {
      return new FakeAudioBufferSourceNode();
    }
    decodeAudioData(_buffer) {
      window.__audioDecodeCalls = (window.__audioDecodeCalls || 0) + 1;
      return Promise.resolve(new FakeAudioBuffer());
    }
    close() {
      this.state = "closed";
      return Promise.resolve();
    }
  }
  window.__audioDecodeCalls = 0;
  window.__audioStartCalls = 0;
  window.__audioStartTimes = [];
  window.AudioContext = FakeAudioContext;
  window.webkitAudioContext = FakeAudioContext;
})();
`;

test.describe("Visual Preview — AI summary speech playback", () => {
	test.use({ viewport: { width: 390, height: 844 } });

	test.beforeEach(async ({ page }) => {
		await page.addInitScript(FAKE_AUDIO_INIT_SCRIPT);

		// Mock article content
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: "https://example.com/speech-test",
				content: "<p>Article body for visual preview speech.</p>",
				articleId: "art-speech-1",
			}),
		);

		// Default mock for StreamSummarize
		await page.route(CONNECT_RPC_PATHS.streamSummarize, (route) =>
			fulfillConnectStream(route, SUMMARY_STREAM_CHUNKS),
		);

		// Default mock for TTS stream
		await page.route(CONNECT_RPC_PATHS.ttsSynthesizeStream, (route) =>
			fulfillConnectStream(route, TTS_THREE_CHUNKS),
		);
	});

	test("speech button is absent during summary streaming and appears after completion", async ({
		page,
	}) => {
		// Delay the summary stream to observe absent state during in-flight streaming
		await page.route(CONNECT_RPC_PATHS.streamSummarize, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillConnectStream(route, SUMMARY_STREAM_CHUNKS);
		});

		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible({ timeout: 15000 });

		// Tap summary button in footer
		const summaryBtn = card.getByRole("button", { name: /summary/i });
		await summaryBtn.click();

		// During summary streaming, the speech button must NOT be rendered
		const speechBtn = card.getByTestId("summary-speech-button");
		await expect(speechBtn).toHaveCount(0);

		// Once the summary completes, speech button appears with label "Play summary"
		await expect(speechBtn).toBeVisible({ timeout: 15000 });
		await expect(speechBtn).toHaveText(/Play summary/i);
	});

	test("tap -> status Playing and 3 decodeAudioData calls", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible({ timeout: 15000 });

		// Request summary
		await card.getByRole("button", { name: /summary/i }).click();

		const speechBtn = card.getByTestId("summary-speech-button");
		await expect(speechBtn).toBeVisible({ timeout: 15000 });

		// Tap play summary
		await speechBtn.click();

		// Status text must indicate Playing
		const statusText = card.getByTestId("summary-speech-status");
		await expect(statusText).toBeVisible();
		await expect(statusText).toHaveText("Playing");

		// Button toggles to "Stop"
		await expect(speechBtn).toHaveText(/Stop/i);

		// Verify 3 decodeAudioData calls occurred on window
		await expect
			.poll(async () => {
				return page.evaluate(
					() =>
						(window as unknown as { __audioDecodeCalls: number })
							.__audioDecodeCalls,
				);
			})
			.toBe(3);
	});

	test("Stop -> back to 'Play summary'", async ({ page }) => {
		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible({ timeout: 15000 });

		await card.getByRole("button", { name: /summary/i }).click();

		const speechBtn = card.getByTestId("summary-speech-button");
		await expect(speechBtn).toBeVisible({ timeout: 15000 });

		// Start playback
		await speechBtn.click();
		await expect(speechBtn).toHaveText(/Stop/i);

		// Tap Stop
		await speechBtn.click();

		// Returns to idle with "Play summary"
		await expect(speechBtn).toHaveText(/Play summary/i);
	});

	test("FailedPrecondition stream error -> 'Speech unavailable', button disabled, summary text still visible", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.ttsSynthesizeStream, (route) =>
			fulfillConnectError(
				route,
				"TTS service disabled",
				"failed_precondition",
				412,
			),
		);

		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible({ timeout: 15000 });

		await card.getByRole("button", { name: /summary/i }).click();

		const speechBtn = card.getByTestId("summary-speech-button");
		await expect(speechBtn).toBeVisible({ timeout: 15000 });

		await speechBtn.click();

		// Status text indicates speech unavailable
		const statusText = card.getByTestId("summary-speech-status");
		await expect(statusText).toBeVisible();
		await expect(statusText).toHaveText("Speech unavailable");

		// Button is permanently disabled (latch)
		await expect(speechBtn).toBeDisabled();

		// AI summary text remains visible
		await expect(
			card.getByText(
				"This is the generated AI summary for visual preview speech test.",
			),
		).toBeVisible();
	});

	test("Unavailable error -> 'Couldn't play the summary. Try again.' and the button works again", async ({
		page,
	}) => {
		let callCount = 0;
		await page.route(CONNECT_RPC_PATHS.ttsSynthesizeStream, async (route) => {
			callCount++;
			if (callCount === 1) {
				await fulfillConnectError(
					route,
					"Upstream service unavailable",
					"unavailable",
					503,
				);
			} else {
				await fulfillConnectStream(route, TTS_THREE_CHUNKS);
			}
		});

		await gotoMobileRoute(page, "/feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible({ timeout: 15000 });

		await card.getByRole("button", { name: /summary/i }).click();

		const speechBtn = card.getByTestId("summary-speech-button");
		await expect(speechBtn).toBeVisible({ timeout: 15000 });

		// First play attempt fails
		await speechBtn.click();

		const statusText = card.getByTestId("summary-speech-status");
		await expect(statusText).toBeVisible();
		await expect(statusText).toHaveText(
			"Couldn't play the summary. Try again.",
		);

		// Button is NOT disabled — retry is allowed
		await expect(speechBtn).toBeEnabled();

		// Second play attempt succeeds
		await speechBtn.click();

		await expect(statusText).toHaveText("Playing");
		await expect(speechBtn).toHaveText(/Stop/i);
	});
});
