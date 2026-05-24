/**
 * Audio utilities for TTS playback.
 *
 * Provides a gapless audio chunk scheduler backed by Web Audio API and a
 * text splitter for the TTS character limit.
 *
 * `createSeamlessTtsPlayer` is the canonical way to play a server-streamed
 * sequence of WAV chunks: each chunk is decoded once via `decodeAudioData`
 * and scheduled onto an AudioBufferSourceNode at sample-accurate end-of-
 * previous-buffer offset, eliminating the per-chunk gap that an
 * `<audio>`-element-per-chunk pipeline produces.
 */

const TTS_MAX_CHARS = 30_000;

/** Small primer to absorb decode + scheduling jitter on the very first chunk. */
const FIRST_CHUNK_PRIMER_SECONDS = 0.05;

export interface SeamlessTtsPlayer {
	/**
	 * Decode the WAV bytes and schedule them on the AudioContext timeline
	 * immediately after the previously scheduled chunk.
	 */
	append(wavBytes: Uint8Array): Promise<void>;
	/** Stop the most recently scheduled chunk and prevent further appends. */
	stop(): void;
	/** Resolve when the last scheduled chunk finishes playing. */
	done(): Promise<void>;
	/** Release the underlying AudioContext. Safe to call multiple times. */
	cleanup(): Promise<void>;
}

type AudioContextCtor = new (options?: AudioContextOptions) => AudioContext;

function resolveAudioContextCtor(): AudioContextCtor {
	const win = globalThis as unknown as {
		AudioContext?: AudioContextCtor;
		webkitAudioContext?: AudioContextCtor;
	};
	const ctor = win.AudioContext ?? win.webkitAudioContext;
	if (!ctor) {
		throw new Error("Web Audio API (AudioContext) is not available");
	}
	return ctor;
}

/**
 * Create a gapless TTS chunk player.
 *
 * Usage:
 *
 *     const player = createSeamlessTtsPlayer();
 *     for await (const chunk of stream) {
 *         await player.append(chunk.audioWav);
 *     }
 *     await player.done();
 *     await player.cleanup();
 */
export function createSeamlessTtsPlayer(): SeamlessTtsPlayer {
	const Ctor = resolveAudioContextCtor();
	const ctx = new Ctor();
	let nextStartTime = 0;
	let lastSource: AudioBufferSourceNode | null = null;
	let stopped = false;
	let endResolve: (() => void) | null = null;
	let pendingChunks = 0;

	const maybeResolveDone = (source: AudioBufferSourceNode | null) => {
		if (!endResolve) return;
		if (pendingChunks > 0) return;
		// Only the most recently appended source signals end-of-playback.
		if (source !== lastSource) return;
		const resolve = endResolve;
		endResolve = null;
		resolve();
	};

	return {
		async append(wavBytes: Uint8Array): Promise<void> {
			if (stopped) return;
			if (wavBytes.length === 0) {
				throw new Error("Empty audio data");
			}
			pendingChunks += 1;
			// decodeAudioData mutates / detaches the buffer it is given; copy
			// the bytes into a private ArrayBuffer so the caller's Uint8Array
			// stays usable for logging / inspection.
			const owned = wavBytes.slice().buffer;
			let buffer: AudioBuffer;
			try {
				buffer = await ctx.decodeAudioData(owned);
			} finally {
				pendingChunks -= 1;
			}
			if (stopped) return;
			const source = ctx.createBufferSource();
			source.buffer = buffer;
			source.connect(ctx.destination);
			const startAt =
				nextStartTime === 0
					? ctx.currentTime + FIRST_CHUNK_PRIMER_SECONDS
					: nextStartTime;
			source.start(startAt);
			nextStartTime = startAt + buffer.duration;
			lastSource = source;
			source.onended = () => maybeResolveDone(source);
		},
		stop(): void {
			stopped = true;
			if (lastSource) {
				try {
					lastSource.stop();
				} catch {
					// already stopped
				}
			}
			if (endResolve) {
				const resolve = endResolve;
				endResolve = null;
				resolve();
			}
		},
		done(): Promise<void> {
			if (stopped) return Promise.resolve();
			// Nothing was ever appended — nothing to wait for.
			if (!lastSource && pendingChunks === 0) return Promise.resolve();
			return new Promise<void>((resolve) => {
				endResolve = resolve;
				// If the last source already ended before done() was called,
				// settle synchronously on the next microtask.
				if (pendingChunks === 0 && lastSource) {
					// Re-arm onended to fire resolve via maybeResolveDone.
					const tail = lastSource;
					tail.onended = () => maybeResolveDone(tail);
				}
			});
		},
		async cleanup(): Promise<void> {
			stopped = true;
			if (ctx.state !== "closed") {
				try {
					await ctx.close();
				} catch {
					// AudioContext already closed
				}
			}
		},
	};
}

/**
 * Splits text into chunks within the TTS character limit.
 *
 * Splits on Japanese period (。) or newline boundaries.
 * Falls back to hard cut if no boundary is found within the limit.
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
		const lastPeriod = window.lastIndexOf("。");
		const lastNewline = window.lastIndexOf("\n");
		const boundary = Math.max(lastPeriod, lastNewline);

		if (boundary > 0) {
			const splitAt = boundary + 1;
			const chunk = remaining.slice(0, splitAt).trim();
			if (chunk.length > 0) {
				chunks.push(chunk);
			}
			remaining = remaining.slice(splitAt).trim();
		} else {
			chunks.push(remaining.slice(0, TTS_MAX_CHARS));
			remaining = remaining.slice(TTS_MAX_CHARS);
		}
	}

	return chunks;
}
