import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillConnectStream, fulfillJson } from "../../utils/mockHelpers";

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

const SUMMARY_STREAM_CHUNKS = [
	{
		chunk: "This is the generated AI summary for visual preview speech test.",
		isFinal: true,
		articleId: "art-speech-1",
		isCached: false,
	},
];

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

test.describe("Feed Detail Modal — AI summary speech playback", () => {
	test.use({ viewport: { width: 390, height: 844 } });

	test.beforeEach(async ({ page }) => {
		await page.addInitScript(FAKE_AUDIO_INIT_SCRIPT);

		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(
			"**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
			(route) => fulfillJson(route, { subscriptions: [] }),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: "https://example.com/speech-test",
				content: "<p>Article body for visual preview speech.</p>",
				articleId: "art-speech-1",
			}),
		);
		await page.route(CONNECT_RPC_PATHS.streamSummarize, (route) =>
			fulfillConnectStream(route, SUMMARY_STREAM_CHUNKS),
		);
		await page.route(CONNECT_RPC_PATHS.ttsSynthesizeStream, (route) =>
			fulfillConnectStream(route, TTS_THREE_CHUNKS),
		);
	});

	test("open tile -> generate summary -> see button -> tap it -> Playing", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/visual-preview");

		const tile = page.getByTestId("gallery-tile").first();
		await expect(tile).toBeVisible({ timeout: 15000 });
		await tile.click();

		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();

		const summarizeBtn = dialog.getByRole("button", { name: /summarize/i });
		await expect(summarizeBtn).toBeEnabled({ timeout: 15000 });
		await summarizeBtn.click();

		const speechBtn = dialog.getByTestId("summary-speech-button");
		await expect(speechBtn).toBeVisible({ timeout: 15000 });
		await expect(speechBtn).toHaveText(/Play summary/i);

		await speechBtn.click();

		const statusText = dialog.getByTestId("summary-speech-status");
		await expect(statusText).toBeVisible();
		await expect(statusText).toHaveText("Playing");

		await expect(speechBtn).toHaveText(/Stop/i);
	});
});
