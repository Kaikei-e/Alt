/**
 * TTS playback hook for Recap genre summaries.
 *
 * Streams server-side WAV chunks (`alt.tts.v1.TTSService/SynthesizeStream`)
 * into a Web-Audio `SeamlessTtsPlayer` that schedules each chunk at
 * sample-accurate end-of-previous offset so playback is gapless across
 * sentence boundaries.
 */

import { createClientTransport, synthesizeSpeechStream } from "$lib/connect";
import {
	createSeamlessTtsPlayer,
	splitTextForTts,
	type SeamlessTtsPlayer,
} from "$lib/utils/audio";

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
	let currentPlayer: SeamlessTtsPlayer | null = null;
	let cancelled = false;

	const stop = () => {
		cancelled = true;
		if (currentPlayer) {
			currentPlayer.stop();
			void currentPlayer.cleanup();
			currentPlayer = null;
		}
		state = "idle";
		error = null;
	};

	const play = async (text: string, options?: TtsPlaybackOptions) => {
		stop();
		cancelled = false;

		if (text.trim().length === 0) {
			state = "idle";
			return;
		}

		const transport = createClientTransport();
		const chunks = splitTextForTts(text);

		let player: SeamlessTtsPlayer | null = null;
		try {
			player = createSeamlessTtsPlayer();
			currentPlayer = player;
			state = "loading";

			for (const textChunk of chunks) {
				if (cancelled) break;

				const stream = synthesizeSpeechStream(transport, {
					text: textChunk,
					voice: options?.voice,
					speed: options?.speed,
				});

				for await (const chunk of stream) {
					if (cancelled) break;
					await player.append(chunk.audioWav);
					if (state === "loading") {
						state = "playing";
					}
				}
			}

			if (!cancelled) {
				await player.done();
			}
		} catch (err) {
			if (!cancelled) {
				error = err instanceof Error ? err.message : "Unknown TTS error";
				state = "error";
				if (player) {
					await player.cleanup();
				}
				if (currentPlayer === player) currentPlayer = null;
				return;
			}
		}

		if (player) {
			await player.cleanup();
		}
		if (currentPlayer === player) currentPlayer = null;

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
