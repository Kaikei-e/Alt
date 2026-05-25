import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type {
	TtsPlaybackStore,
	TtsTrack,
} from "$lib/stores/ttsPlayback.svelte";
import TtsMiniPlayer from "./TtsMiniPlayer.svelte";

function makeStore(
	overrides: Partial<TtsPlaybackStore> = {},
): TtsPlaybackStore {
	return {
		state: "idle",
		isPlaying: false,
		isLoading: false,
		isActive: false,
		error: null,
		track: null,
		play: vi.fn(),
		stop: vi.fn(),
		...overrides,
	} as TtsPlaybackStore;
}

const ACTIVE_TRACK: TtsTrack = {
	articleId: "a",
	title: "An article about clean architecture",
	source: "body",
};

describe("TtsMiniPlayer", () => {
	it("renders nothing when the store is idle", async () => {
		render(TtsMiniPlayer, { props: { store: makeStore() } });
		// No region/landmark is rendered while idle.
		const regionLocator = page.getByRole("region", {
			name: /audio playback/i,
		});
		await expect.element(regionLocator).not.toBeInTheDocument();
	});

	it("renders the track title when active", async () => {
		const store = makeStore({
			state: "playing",
			isPlaying: true,
			isActive: true,
			track: ACTIVE_TRACK,
		});
		render(TtsMiniPlayer, { props: { store } });

		await expect
			.element(page.getByText(/clean architecture/i))
			.toBeInTheDocument();
	});

	it("shows a Pause button while playing", async () => {
		const store = makeStore({
			state: "playing",
			isPlaying: true,
			isActive: true,
			track: ACTIVE_TRACK,
		});
		render(TtsMiniPlayer, { props: { store } });

		await expect
			.element(page.getByRole("button", { name: /pause/i }))
			.toBeInTheDocument();
	});

	it("close button calls store.stop()", async () => {
		const stop = vi.fn();
		const store = makeStore({
			state: "playing",
			isPlaying: true,
			isActive: true,
			track: ACTIVE_TRACK,
			stop,
		});
		render(TtsMiniPlayer, { props: { store } });

		await page.getByRole("button", { name: /close/i }).click();
		expect(stop).toHaveBeenCalled();
	});

	it("region has an accessible label", async () => {
		const store = makeStore({
			state: "playing",
			isPlaying: true,
			isActive: true,
			track: ACTIVE_TRACK,
		});
		render(TtsMiniPlayer, { props: { store } });

		await expect
			.element(page.getByRole("region", { name: /audio playback/i }))
			.toBeInTheDocument();
	});
});
