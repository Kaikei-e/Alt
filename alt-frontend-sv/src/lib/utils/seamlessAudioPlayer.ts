/**
 * Web Audio gapless chunk player.
 *
 * Enqueues audio WAV chunks, decodes them via AudioContext.decodeAudioData,
 * and schedules AudioBufferSourceNode instances back-to-back without gaps.
 */

const FIRST_CHUNK_PRIMER_SECONDS = 0.05;

export interface SeamlessAudioPlayer {
	/** Callers must await enqueue in order to maintain gapless playback timing. */
	enqueue(wav: Uint8Array): Promise<void>;
	stop(): void;
	waitUntilDrained(): Promise<void>;
}

/**
 * Creates a gapless audio player attached to the given AudioContext.
 */
export function createSeamlessAudioPlayer(
	context: AudioContext,
): SeamlessAudioPlayer {
	let nextStartTime = 0;
	let stopped = false;
	let pendingDecodes = 0;
	const activeSources = new Set<AudioBufferSourceNode>();
	const drainResolvers: Array<() => void> = [];

	function checkDrain() {
		if (activeSources.size === 0 && pendingDecodes === 0) {
			const resolvers = drainResolvers.splice(0);
			for (const resolve of resolvers) {
				resolve();
			}
		}
	}

	return {
		async enqueue(wav: Uint8Array): Promise<void> {
			if (stopped) return;
			if (wav.length === 0) return;

			pendingDecodes++;
			const owned = wav.buffer.slice(
				wav.byteOffset,
				wav.byteOffset + wav.byteLength,
			) as ArrayBuffer;
			let buffer: AudioBuffer;
			try {
				buffer = await context.decodeAudioData(owned);
			} finally {
				pendingDecodes--;
			}

			if (stopped) {
				checkDrain();
				return;
			}

			const source = context.createBufferSource();
			source.buffer = buffer;
			source.connect(context.destination);

			const startAt =
				nextStartTime === 0
					? context.currentTime + FIRST_CHUNK_PRIMER_SECONDS
					: Math.max(nextStartTime, context.currentTime);

			source.start(startAt);
			nextStartTime = startAt + buffer.duration;

			activeSources.add(source);
			source.onended = () => {
				activeSources.delete(source);
				checkDrain();
			};
		},

		stop(): void {
			stopped = true;
			const sourcesToStop = Array.from(activeSources);
			activeSources.clear();
			for (const source of sourcesToStop) {
				try {
					source.stop();
				} catch {
					// already stopped
				}
			}
			const resolvers = drainResolvers.splice(0);
			for (const resolve of resolvers) {
				resolve();
			}
		},

		waitUntilDrained(): Promise<void> {
			if (stopped || (activeSources.size === 0 && pendingDecodes === 0)) {
				return Promise.resolve();
			}
			return new Promise<void>((resolve) => {
				drainResolvers.push(resolve);
			});
		},
	};
}
