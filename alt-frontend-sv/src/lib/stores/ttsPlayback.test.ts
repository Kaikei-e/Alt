import { beforeEach, describe, expect, it, vi } from "vitest";

// Mock the audio + connect modules BEFORE importing the store under test.
// The fake player lets each test drive append/done/stop deterministically
// without touching Web Audio or the network.

interface FakePlayerControls {
	resolveDone(): void;
	rejectDecode(err: Error): void;
}

let currentFakePlayer: ReturnType<typeof makeFakePlayer> | null = null;

function makeFakePlayer() {
	let donePromiseResolve: (() => void) | null = null;
	let appendShouldReject: Error | null = null;
	let stopped = false;
	const calls: { type: "append" | "stop" | "cleanup" | "done" }[] = [];

	const api = {
		append: vi.fn(async (_bytes: Uint8Array) => {
			calls.push({ type: "append" });
			if (appendShouldReject) {
				const err = appendShouldReject;
				appendShouldReject = null;
				throw err;
			}
		}),
		stop: vi.fn(() => {
			calls.push({ type: "stop" });
			stopped = true;
			// Match real SeamlessTtsPlayer: a pending done() resolves on stop().
			if (donePromiseResolve) {
				const r = donePromiseResolve;
				donePromiseResolve = null;
				r();
			}
		}),
		done: vi.fn(() => {
			calls.push({ type: "done" });
			if (stopped) return Promise.resolve();
			return new Promise<void>((resolve) => {
				donePromiseResolve = resolve;
			});
		}),
		cleanup: vi.fn(async () => {
			calls.push({ type: "cleanup" });
		}),
	};

	const controls: FakePlayerControls = {
		resolveDone() {
			donePromiseResolve?.();
		},
		rejectDecode(err) {
			appendShouldReject = err;
		},
	};

	return { api, controls, calls };
}

vi.mock("$lib/utils/audio", async () => {
	const actual =
		await vi.importActual<typeof import("$lib/utils/audio")>(
			"$lib/utils/audio",
		);
	return {
		...actual,
		createSeamlessTtsPlayer: () => {
			currentFakePlayer = makeFakePlayer();
			return currentFakePlayer.api;
		},
	};
});

let nextStreamChunks: Uint8Array[] = [new Uint8Array([1, 2, 3])];
let streamShouldThrow: Error | null = null;

vi.mock("$lib/connect", () => ({
	createClientTransport: () => ({}),
	synthesizeSpeechStream: async function* () {
		if (streamShouldThrow) {
			const err = streamShouldThrow;
			streamShouldThrow = null;
			throw err;
		}
		for (const bytes of nextStreamChunks) {
			yield {
				audioWav: bytes,
				sampleRate: 24000,
				durationSeconds: 1.0,
			};
		}
	},
}));

// Importing after the mocks so the store sees the stubbed modules.
import {
	createTtsPlaybackStore,
	TTS_PLAYBACK_KEY,
	type TtsPlaybackStore,
	type TtsTrack,
} from "./ttsPlayback.svelte";

const TRACK_SUMMARY: TtsTrack = {
	articleId: "art-1",
	title: "An article",
	source: "summary",
};

const TRACK_BODY: TtsTrack = {
	articleId: "art-2",
	title: "Another article",
	source: "body",
};

describe("TtsPlaybackStore", () => {
	let store: TtsPlaybackStore;

	beforeEach(() => {
		nextStreamChunks = [new Uint8Array([1, 2, 3])];
		streamShouldThrow = null;
		currentFakePlayer = null;
		store = createTtsPlaybackStore();
	});

	describe("initial state", () => {
		it("starts idle with no track and no error", () => {
			expect(store.state).toBe("idle");
			expect(store.isPlaying).toBe(false);
			expect(store.isLoading).toBe(false);
			expect(store.isActive).toBe(false);
			expect(store.track).toBeNull();
			expect(store.error).toBeNull();
		});
	});

	describe("play()", () => {
		it("sets track and transitions loading → playing → idle on a normal stream", async () => {
			const playPromise = store.play(TRACK_SUMMARY, "hello world");
			// Yield once so the loading state lands before the first chunk arrives.
			await Promise.resolve();
			expect(store.track).toEqual(TRACK_SUMMARY);

			// Wait until the store has reached `await player.done()` before
			// resolving it — `vi.waitFor` polls without coupling the test to
			// the exact microtask count in the play loop.
			await vi.waitFor(() => {
				expect(currentFakePlayer?.api.done).toHaveBeenCalled();
			});
			currentFakePlayer?.controls.resolveDone();
			await playPromise;
			expect(store.state).toBe("idle");
			expect(store.track).toBeNull();
			expect(currentFakePlayer?.api.cleanup).toHaveBeenCalled();
		});

		it("becomes a no-op for empty text", async () => {
			await store.play(TRACK_SUMMARY, "   ");
			expect(store.state).toBe("idle");
			expect(store.track).toBeNull();
			expect(currentFakePlayer).toBeNull();
		});

		it("transitions to error when the stream throws", async () => {
			streamShouldThrow = new Error("boom");
			await store.play(TRACK_BODY, "content");
			expect(store.state).toBe("error");
			expect(store.error).toBe("boom");
		});

		it("cancels a previous playback when called again", async () => {
			const first = store.play(TRACK_SUMMARY, "first text");
			await Promise.resolve();
			const firstPlayer = currentFakePlayer;
			expect(firstPlayer).not.toBeNull();

			// Replace before the first finishes. The second play() bumps the
			// active token so the first loop becomes a no-op.
			const second = store.play(TRACK_BODY, "second text");
			expect(firstPlayer?.api.stop).toHaveBeenCalled();
			expect(currentFakePlayer).not.toBe(firstPlayer);

			await vi.waitFor(() => {
				expect(currentFakePlayer?.api.done).toHaveBeenCalled();
			});
			currentFakePlayer?.controls.resolveDone();
			await Promise.all([first, second]);
			expect(store.track).toBeNull();
		});
	});

	describe("stop()", () => {
		it("resets state and clears the track immediately", async () => {
			const p = store.play(TRACK_SUMMARY, "some text");
			await Promise.resolve();
			expect(store.track).toEqual(TRACK_SUMMARY);
			const playerWhilePlaying = currentFakePlayer;
			store.stop();
			expect(store.state).toBe("idle");
			expect(store.track).toBeNull();
			expect(playerWhilePlaying?.api.stop).toHaveBeenCalled();
			await p;
		});

		it("is safe to call when nothing is playing", () => {
			expect(() => store.stop()).not.toThrow();
			expect(store.state).toBe("idle");
		});
	});

	describe("context key", () => {
		it("exports a unique Symbol for context key", () => {
			expect(typeof TTS_PLAYBACK_KEY).toBe("symbol");
		});
	});
});
