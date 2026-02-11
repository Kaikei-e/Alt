/**
 * TTS playback hook for Recap genre summaries.
 *
 * Manages the lifecycle of text-to-speech synthesis and audio playback.
 * Uses server-streaming RPC — the server chunks text by sentence and yields
 * complete WAV files, which we play back sequentially.
 */

import { createClientTransport, synthesizeSpeechStream } from "$lib/connect";
import { createAudioFromWav } from "$lib/utils/audio";
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

		if (text.trim().length === 0) {
			state = "idle";
			return;
		}

		const transport = createClientTransport();

		try {
			state = "loading";
			const stream = synthesizeSpeechStream(transport, {
				text,
				voice: options?.voice,
				speed: options?.speed,
			});

			for await (const chunk of stream) {
				if (cancelled) break;

				const player = createAudioFromWav(chunk.audioWav);
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
			}
		} catch (err) {
			if (!cancelled) {
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
