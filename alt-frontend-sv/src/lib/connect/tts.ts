/**
 * TTSService client for Connect-RPC
 *
 * Provides streaming speech synthesis over alt.tts.v1.TTSService.
 */

import type { Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import { TTSService } from "$lib/gen/alt/tts/v1/tts_pb";

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
export async function* synthesizeSpeechStream(
	transport: Transport,
	options: SynthesizeSpeechStreamOptions,
	signal?: AbortSignal,
): AsyncGenerator<SpeechChunk> {
	const client = createClient(TTSService, transport);
	const stream = client.synthesizeStream(
		{
			text: options.text,
			speed: options.speed,
		},
		{ signal },
	);

	for await (const response of stream) {
		yield {
			audioWav: response.audioWav,
			sampleRate: response.sampleRate,
			durationSeconds: response.durationSeconds,
		};
	}
}
