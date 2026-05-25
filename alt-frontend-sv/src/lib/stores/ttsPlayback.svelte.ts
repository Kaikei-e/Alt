/**
 * Global TTS playback store. Lives in the (app) layout so a single
 * mini-player can survive route changes — exactly one playback at a time
 * across the whole app, matching the user-perceived mental model.
 *
 * The playback loop mirrors `useTtsPlayback` (kept for desktop callers in
 * v1) and is the next refactor target once both surfaces are wired.
 */

import { getContext } from "svelte";
import {
	createSeamlessTtsPlayer,
	type SeamlessTtsPlayer,
} from "$lib/utils/audio";
import { runTtsStreamLoop } from "$lib/utils/ttsStream";

export type TtsState = "idle" | "loading" | "playing" | "error";
export type TtsSource = "summary" | "body";

export interface TtsTrack {
	articleId: string;
	title: string;
	source: TtsSource;
}

export interface TtsPlaybackOptions {
	voice?: string;
	speed?: number;
}

export interface TtsPlaybackStore {
	readonly state: TtsState;
	readonly isPlaying: boolean;
	readonly isLoading: boolean;
	readonly isActive: boolean;
	readonly error: string | null;
	readonly track: TtsTrack | null;
	play(
		track: TtsTrack,
		text: string,
		options?: TtsPlaybackOptions,
	): Promise<void>;
	stop(): void;
}

export const TTS_PLAYBACK_KEY = Symbol("tts-playback");

class TtsPlaybackStoreImpl implements TtsPlaybackStore {
	private _state = $state<TtsState>("idle");
	private _error = $state<string | null>(null);
	private _track = $state<TtsTrack | null>(null);
	private _currentPlayer: SeamlessTtsPlayer | null = null;
	// Per-invocation token: bumped on every stop()/play() entry so a stale
	// loop knows it has been superseded and stops mutating shared state.
	private _activeToken = 0;

	readonly isPlaying = $derived(this._state === "playing");
	readonly isLoading = $derived(this._state === "loading");
	readonly isActive = $derived(
		this._state === "playing" || this._state === "loading",
	);

	get state(): TtsState {
		return this._state;
	}

	get error(): string | null {
		return this._error;
	}

	get track(): TtsTrack | null {
		return this._track;
	}

	stop(): void {
		this._activeToken += 1;
		if (this._currentPlayer) {
			this._currentPlayer.stop();
			void this._currentPlayer.cleanup();
			this._currentPlayer = null;
		}
		this._state = "idle";
		this._error = null;
		this._track = null;
	}

	async play(
		track: TtsTrack,
		text: string,
		options?: TtsPlaybackOptions,
	): Promise<void> {
		this.stop();
		const myToken = ++this._activeToken;
		const isActive = () => this._activeToken === myToken;

		if (text.trim().length === 0) {
			return;
		}

		this._track = track;

		let player: SeamlessTtsPlayer | null = null;
		try {
			player = createSeamlessTtsPlayer();
			this._currentPlayer = player;
			this._state = "loading";

			await runTtsStreamLoop({
				player,
				text,
				voice: options?.voice,
				speed: options?.speed,
				isCancelled: () => !isActive(),
				onChunkAppended: () => {
					if (isActive() && this._state === "loading") {
						this._state = "playing";
					}
				},
			});
		} catch (err) {
			if (isActive()) {
				this._error = err instanceof Error ? err.message : "Unknown TTS error";
				this._state = "error";
				this._track = null;
				if (player) {
					await player.cleanup();
				}
				if (this._currentPlayer === player) this._currentPlayer = null;
				return;
			}
		}

		if (player) {
			await player.cleanup();
		}
		if (this._currentPlayer === player) this._currentPlayer = null;

		if (isActive()) {
			this._state = "idle";
			this._track = null;
		}
	}
}

export function createTtsPlaybackStore(): TtsPlaybackStore {
	return new TtsPlaybackStoreImpl();
}

export function getTtsPlaybackStore(): TtsPlaybackStore {
	return getContext<TtsPlaybackStore>(TTS_PLAYBACK_KEY);
}
