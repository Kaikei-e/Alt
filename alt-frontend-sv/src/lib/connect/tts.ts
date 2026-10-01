/**
 * TTSService client for Connect-RPC
 *
 * Provides streaming speech synthesis over alt.tts.v1.TTSService.
 */

import type { Transport } from "@connectrpc/connect";

export interface SynthesizeSpeechStreamOptions {
	text: string;
	speed?: number;
}

export interface SpeechChunk {
	audioWav: Uint8Array;
	sampleRate: number;
	durationSeconds: number;
}

/**
 * Synthesizes speech via server-streaming Connect-RPC.
 *
 * Yields audio chunks as self-contained WAV binaries.
 */
// biome-ignore lint/correctness/useYield: minimal stub for TDD RED phase
export async function* synthesizeSpeechStream(
	_transport: Transport,
	_options: SynthesizeSpeechStreamOptions,
	_signal?: AbortSignal,
): AsyncGenerator<SpeechChunk> {
	throw new Error("not implemented");
}
