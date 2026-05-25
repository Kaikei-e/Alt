import { expect, test } from "@playwright/test";
import { CONNECT_RPC_PATHS } from "../../fixtures/mockData";
import { fulfillConnectStream, fulfillJson } from "../../utils/mockHelpers";

const TTS_STREAM_PATH = "**/api/v2/alt.tts.v1.TTSService/SynthesizeStream";

const ARTICLE_URL = "https://example.com/an-article";
const ARTICLE_TITLE = "Reading TTS on Mobile";
const ARTICLE_ID = "art-tts-1";

/**
 * Inject a fake Web Audio API so the TTS pipeline can run without decoding
 * real WAV bytes in headless Chromium. The server-side TTS path goes through
 * SeamlessTtsPlayer which calls decodeAudioData — that would fail on the
 * stub bytes we ship from the mocked Connect stream.
 */
const FAKE_AUDIO_INIT_SCRIPT = `
(function () {
  // Long buffer + onended that only fires when stop() is explicitly called.
  // This keeps the playback loop pinned in the "playing" state so the UI
  // assertion has time to observe it; close() / store.stop() is the only
  // path out, which is what the test exercises.
  class FakeAudioBuffer { constructor(){ this.duration = 600; } }
  class FakeAudioBufferSourceNode {
    constructor(){ this.buffer = null; this.onended = null; this._stopped = false; }
    connect(){}
    start(){ /* no auto-fire — onended only triggers from stop() */ }
    stop(){
      if (this._stopped) return;
      this._stopped = true;
      setTimeout(() => { try { this.onended && this.onended(); } catch(_){} }, 0);
    }
  }
  class FakeAudioContext {
    constructor(){ this.state = "running"; this.currentTime = 0; this.destination = {}; }
    createBufferSource(){ return new FakeAudioBufferSourceNode(); }
    decodeAudioData(_buffer){ return Promise.resolve(new FakeAudioBuffer()); }
    close(){ this.state = "closed"; return Promise.resolve(); }
  }
  window.AudioContext = FakeAudioContext;
  window.webkitAudioContext = FakeAudioContext;
})();
`;

test.describe("Mobile TTS — article reader", () => {
	test.use({ viewport: { width: 390, height: 844 } });

	test.beforeEach(async ({ page }) => {
		await page.addInitScript(FAKE_AUDIO_INIT_SCRIPT);

		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: ARTICLE_URL,
				content: "<p>Body content for TTS. Plenty of words to read aloud.</p>",
				articleId: ARTICLE_ID,
			}),
		);

		// Mock the TTS stream — a single short chunk is enough for the UI flow.
		await page.route(TTS_STREAM_PATH, (route) =>
			fulfillConnectStream(route, [
				{
					audioWav: Buffer.from([0, 0, 0, 0]).toString("base64"),
					sampleRate: 24000,
					durationSeconds: 0.1,
				},
			]),
		);
	});

	test("opens overflow → setup sheet → mini-player persists across navigation", async ({
		page,
	}) => {
		const url = `/articles/${ARTICLE_ID}?url=${encodeURIComponent(
			ARTICLE_URL,
		)}&title=${encodeURIComponent(ARTICLE_TITLE)}`;
		await page.goto(url);

		// Wait for article body to load before interacting with the menu.
		await expect(page.getByTestId("article-content-surface")).toBeVisible();

		// Open the overflow menu in the action bar.
		await page.getByRole("button", { name: /article actions/i }).click();

		// Select "Listen to article".
		await page.getByRole("menuitem", { name: /listen to article/i }).click();

		// Setup sheet is now visible. Pick the Article (body) source.
		const articleRadio = page.getByRole("radio", { name: /article/i });
		await expect(articleRadio).toBeVisible();
		await articleRadio.click();

		// Start playback. This is the user gesture that unlocks AudioContext.
		await page.getByRole("button", { name: /^start/i }).click();

		// Mini-player is rendered as a region landmark.
		const miniPlayer = page.getByRole("region", {
			name: /audio playback/i,
		});
		await expect(miniPlayer).toBeVisible();

		// SPA-navigate away via the bottom-nav. A hard `page.goto` would tear
		// down the (app) layout and recreate the store — we need to prove the
		// mini-player survives a client-side route change.
		await page
			.getByRole("navigation", { name: /main navigation/i })
			.getByRole("link", { name: "Home" })
			.click();
		await expect(page).toHaveURL(/\/home/);
		await expect(miniPlayer).toBeVisible();

		// Close stops playback and dismisses the mini-player.
		await miniPlayer.getByRole("button", { name: /close/i }).click();
		await expect(miniPlayer).toHaveCount(0);
	});
});
