/**
 * TTSService client for Connect-RPC
 *
 * Provides type-safe methods to call TTSService endpoints.
 * Authentication is handled by the transport layer.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	TTSService,
	type SynthesizeResponse,
	type SynthesizeStreamResponse,
	type ListVoicesResponse,
} from "$lib/gen/alt/tts/v1/tts_pb";

/** Type-safe TTSService client */
type TtsClient = Client<typeof TTSService>;

/** Result from a synthesize call */
export interface SynthesizeResult {
	audioWav: Uint8Array;
	sampleRate: number;
	durationSeconds: number;
}

/** Voice information */
export interface TtsVoice {
	id: string;
	name: string;
	gender: string;
}

/** Options for synthesize call */
export interface SynthesizeOptions {
	text: string;
	voice?: string;
	speed?: number;
}

const DEFAULT_VOICE = "jf_gongitsune";
const DEFAULT_SPEED = 1.25;

/**
 * Creates a TTSService client with the given transport.
 */
export function createTtsClient(transport: Transport): TtsClient {
	return createClient(TTSService, transport);
}

/**
 * Synthesizes speech from text via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param options - Text and optional voice/speed settings
 * @returns Synthesized audio data
 */
export async function synthesizeSpeech(
	transport: Transport,
	options: SynthesizeOptions,
): Promise<SynthesizeResult> {
	const client = createTtsClient(transport);
	const response = (await client.synthesize({
		text: options.text,
		voice: options.voice ?? DEFAULT_VOICE,
		speed: options.speed ?? DEFAULT_SPEED,
	})) as SynthesizeResponse;

	return {
		audioWav: response.audioWav,
		sampleRate: response.sampleRate,
		durationSeconds: response.durationSeconds,
	};
}

/**
 * Synthesizes speech via server-streaming RPC.
 *
 * The server yields one SynthesizeResponse per sentence chunk (each a complete WAV).
 *
 * @param transport - The Connect transport to use
 * @param options - Text and optional voice/speed settings
 * @returns AsyncIterable of SynthesizeResult chunks
 */
export async function* synthesizeSpeechStream(
	transport: Transport,
	options: SynthesizeOptions,
): AsyncGenerator<SynthesizeResult> {
	const client = createTtsClient(transport);
	const stream = client.synthesizeStream({
		text: options.text,
		voice: options.voice ?? DEFAULT_VOICE,
		speed: options.speed ?? DEFAULT_SPEED,
	});

	for await (const rawResponse of stream) {
		const response = rawResponse as SynthesizeStreamResponse;
		yield {
			audioWav: response.audioWav,
			sampleRate: response.sampleRate,
			durationSeconds: response.durationSeconds,
		};
	}
}

/**
 * Lists available TTS voices via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @returns Array of available voices
 */
export async function listVoices(transport: Transport): Promise<TtsVoice[]> {
	const client = createTtsClient(transport);
	const response = (await client.listVoices({})) as ListVoicesResponse;

	return response.voices.map((v) => ({
		id: v.id,
		name: v.name,
		gender: v.gender,
	}));
}
