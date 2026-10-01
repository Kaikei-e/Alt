/**
 * Web Audio gapless chunk player.
 *
 * Enqueues audio WAV chunks, decodes them via AudioContext.decodeAudioData,
 * and schedules AudioBufferSourceNode instances back-to-back without gaps.
 */

export interface SeamlessAudioPlayer {
	enqueue(wav: Uint8Array): Promise<void>;
	stop(): void;
	waitUntilDrained(): Promise<void>;
}

/**
 * Creates a gapless audio player attached to the given AudioContext.
 */
export function createSeamlessAudioPlayer(
	_context: AudioContext,
): SeamlessAudioPlayer {
	throw new Error("not implemented");
}
