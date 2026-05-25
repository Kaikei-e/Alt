/**
 * Shared TTS streaming loop — used by both the per-component `useTtsPlayback`
 * hook (desktop) and the singleton `TtsPlaybackStore` (mobile).
 *
 * Responsibilities:
 * - Split the text via `splitTextForTts` so each request stays under the
 *   server limit.
 * - For each chunk, open a server-streaming RPC and pipe each WAV chunk into
 *   the player.
 * - Honour caller-driven cancellation between every await point so a stale
 *   loop never mutates state owned by the next play.
 *
 * State and player lifecycle are intentionally left to the caller — this
 * helper only orchestrates the network → player handoff.
 */

import { createClientTransport, synthesizeSpeechStream } from "$lib/connect";
import { type SeamlessTtsPlayer, splitTextForTts } from "./audio";

export interface TtsStreamLoopOptions {
	player: SeamlessTtsPlayer;
	text: string;
	voice?: string;
	speed?: number;
	/** Return `true` to abort the loop at the next safe point. */
	isCancelled: () => boolean;
	/** Fires after each successful `player.append` so callers can flip state. */
	onChunkAppended?: () => void;
}

export async function runTtsStreamLoop(
	options: TtsStreamLoopOptions,
): Promise<void> {
	const { player, text, voice, speed, isCancelled, onChunkAppended } = options;

	const transport = createClientTransport();
	const chunks = splitTextForTts(text);

	for (const textChunk of chunks) {
		if (isCancelled()) return;

		const stream = synthesizeSpeechStream(transport, {
			text: textChunk,
			voice,
			speed,
		});

		for await (const chunk of stream) {
			if (isCancelled()) return;
			await player.append(chunk.audioWav);
			onChunkAppended?.();
		}
	}

	if (!isCancelled()) {
		await player.done();
	}
}
