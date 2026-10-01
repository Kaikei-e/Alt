/**
 * Hook for playing AI summary speech.
 *
 * Manages playback state machine (idle | loading | playing | error | unavailable),
 * AudioContext gesture initialization, stream cancellation, and error latching.
 */

import type { Transport } from "@connectrpc/connect";
import type {
	SpeechChunk,
	SynthesizeSpeechStreamOptions,
} from "$lib/connect/tts";
import type { SeamlessAudioPlayer } from "$lib/utils/seamlessAudioPlayer";

export type SummarySpeechState =
	| "idle"
	| "loading"
	| "playing"
	| "error"
	| "unavailable";

export interface SummarySpeechDeps {
	transport?: Transport;
	audioContext?: AudioContext;
	createAudioContext?: () => AudioContext;
	createPlayer?: (context: AudioContext) => SeamlessAudioPlayer;
	synthesizeStream?: (
		transport: Transport,
		options: SynthesizeSpeechStreamOptions,
		signal?: AbortSignal,
	) => AsyncIterable<SpeechChunk>;
}

export interface SummarySpeech {
	readonly state: SummarySpeechState;
	play(text: string): Promise<void> | void;
	stop(): void;
}

/**
 * Creates a reactive speech player controller for Visual Preview AI summaries.
 */
export function createSummarySpeech(_deps?: SummarySpeechDeps): SummarySpeech {
	throw new Error("not implemented");
}
