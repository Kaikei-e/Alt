/**
 * Audio utilities for TTS playback.
 *
 * Provides WAV audio player creation and text splitting for TTS character limits.
 */

const TTS_MAX_CHARS = 5000;

export interface AudioPlayer {
	play(): Promise<void>;
	stop(): void;
	cleanup(): void;
	onEnded(callback: () => void): void;
}

/**
 * Creates an AudioPlayer from WAV binary data.
 *
 * @param wavBytes - Raw WAV audio bytes
 * @returns AudioPlayer with play/stop/cleanup controls
 * @throws Error if wavBytes is empty
 */
export function createAudioFromWav(wavBytes: Uint8Array): AudioPlayer {
	if (wavBytes.length === 0) {
		throw new Error("Empty audio data");
	}

	const blob = new Blob([wavBytes as BlobPart], { type: "audio/wav" });
	const url = URL.createObjectURL(blob);
	const audio = new Audio(url);

	return {
		play() {
			return audio.play();
		},
		stop() {
			audio.pause();
			audio.currentTime = 0;
		},
		cleanup() {
			URL.revokeObjectURL(url);
		},
		onEnded(callback: () => void) {
			audio.addEventListener("ended", callback, { once: true });
		},
	};
}

/**
 * Splits text into chunks within the TTS character limit.
 *
 * Splits on Japanese period (。) or newline boundaries.
 * Falls back to hard cut if no boundary is found within the limit.
 *
 * @param text - Text to split
 * @returns Array of text chunks, each within TTS_MAX_CHARS
 */
export function splitTextForTts(text: string): string[] {
	const trimmed = text.trim();
	if (trimmed.length === 0) {
		return [];
	}

	if (trimmed.length <= TTS_MAX_CHARS) {
		return [trimmed];
	}

	const chunks: string[] = [];
	let remaining = trimmed;

	while (remaining.length > 0) {
		if (remaining.length <= TTS_MAX_CHARS) {
			chunks.push(remaining);
			break;
		}

		const window = remaining.slice(0, TTS_MAX_CHARS);
		// Find the last sentence boundary within the window
		const lastPeriod = window.lastIndexOf("。");
		const lastNewline = window.lastIndexOf("\n");
		const boundary = Math.max(lastPeriod, lastNewline);

		if (boundary > 0) {
			// Split at boundary (include the delimiter in the chunk)
			const splitAt = boundary + 1;
			const chunk = remaining.slice(0, splitAt).trim();
			if (chunk.length > 0) {
				chunks.push(chunk);
			}
			remaining = remaining.slice(splitAt).trim();
		} else {
			// Hard cut at limit
			chunks.push(remaining.slice(0, TTS_MAX_CHARS));
			remaining = remaining.slice(TTS_MAX_CHARS);
		}
	}

	return chunks;
}
