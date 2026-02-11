/**
 * TTS playback hook for Recap genre summaries.
 *
 * Manages the lifecycle of text-to-speech synthesis and audio playback.
 * Supports chunked playback for long texts and cleanup on genre changes.
 */

import { createClientTransport, synthesizeSpeech } from "$lib/connect";
import { createAudioFromWav, splitTextForTts } from "$lib/utils/audio";
import type { AudioPlayer } from "$lib/utils/audio";

type TtsState = "idle" | "loading" | "playing" | "error";

interface TtsPlaybackOptions {
	voice?: string;
	speed?: number;
}

export interface TtsPlayback {
	readonly state: TtsState;
	readonly isPlaying: boolean;
	readonly isLoading: boolean;
	readonly error: string | null;
	play(text: string, options?: TtsPlaybackOptions): Promise<void>;
	stop(): void;
}

export function useTtsPlayback(): TtsPlayback {
	let state = $state<TtsState>("idle");
	let error = $state<string | null>(null);
	let currentPlayer: AudioPlayer | null = null;
	let cancelled = false;
	let pendingResolve: (() => void) | null = null;

	const stop = () => {
		cancelled = true;
		if (currentPlayer) {
			currentPlayer.stop();
			currentPlayer.cleanup();
			currentPlayer = null;
		}
		// Resolve any pending onEnded promise so play() can exit
		if (pendingResolve) {
			pendingResolve();
			pendingResolve = null;
		}
		state = "idle";
		error = null;
	};

	const play = async (text: string, options?: TtsPlaybackOptions) => {
		// Stop any existing playback
		stop();
		cancelled = false;

		const chunks = splitTextForTts(text);
		if (chunks.length === 0) {
			state = "idle";
			return;
		}

		const transport = createClientTransport();

		for (const chunk of chunks) {
			if (cancelled) break;

			try {
				state = "loading";
				const result = await synthesizeSpeech(transport, {
					text: chunk,
					voice: options?.voice,
					speed: options?.speed,
				});

				if (cancelled) break;

				const player = createAudioFromWav(result.audioWav);
				currentPlayer = player;
				state = "playing";

				await player.play();

				// Wait for playback to finish or cancellation
				await new Promise<void>((resolve) => {
					pendingResolve = resolve;
					player.onEnded(() => {
						pendingResolve = null;
						resolve();
					});
				});

				if (!cancelled) {
					player.cleanup();
					currentPlayer = null;
				}
			} catch (err) {
				if (cancelled) break;
				error = err instanceof Error ? err.message : "Unknown TTS error";
				state = "error";
				return;
			}
		}

		if (!cancelled) {
			state = "idle";
		}
	};

	return {
		get state() {
			return state;
		},
		get isPlaying() {
			return state === "playing";
		},
		get isLoading() {
			return state === "loading";
		},
		get error() {
			return error;
		},
		play,
		stop,
	};
}
