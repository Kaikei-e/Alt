/**
 * Utility for incremental rendering of streaming text data
 * Uses Svelte's reactivity system for immediate UI updates
 * Based on best practices for streaming text rendering
 */

export interface StreamingRendererOptions {
	/**
	 * Callback for logging (optional)
	 */
	onChunk?: (
		chunkCount: number,
		chunkSize: number,
		decodedLength: number,
		totalLength: number,
		preview: string,
	) => void;
	/**
	 * Callback when stream completes
	 */
	onComplete?: (totalLength: number, chunkCount: number) => void;
	/**
	 * Callback for metadata events (e.g. citations, context)
	 */
	onMetadata?: (metadata: any) => void;
	/**
	 * Optional tick function from Svelte to force re-render
	 * If provided, will be called after each chunk to ensure immediate rendering
	 */
	tick?: () => Promise<void>;
	/**
	 * Enable typewriter effect - render text character by character
	 * @default false
	 */
	typewriter?: boolean;
	/**
	 * Delay between each character in milliseconds (only used when typewriter is enabled)
	 * @default 10
	 */
	typewriterDelay?: number;
}

/**
 * Creates a streaming renderer that immediately updates state for each chunk
 * This leverages Svelte's reactivity system for incremental rendering
 * Supports optional typewriter effect for character-by-character rendering
 */
export function simulateTypewriterEffect(
	onChar: (char: string) => void,
	options: {
		tick?: () => Promise<void>;
		delay?: number;
	} = {},
) {
	const { tick, delay = 10 } = options;
	let queue = Promise.resolve();
	let isCancelled = false;

	const add = (newText: string) => {
		if (!newText || isCancelled) return;

		// Queue typewriter rendering to prevent overlapping
		queue = queue
			.then(async () => {
				if (isCancelled) return;

				try {
					for (let i = 0; i < newText.length; i++) {
						if (isCancelled) break;
						onChar(newText[i]);

						// Wait for delay
						if (i < newText.length - 1 && delay > 0) {
							await new Promise<void>((resolve) => setTimeout(resolve, delay));
						}

						// Call tick periodically
						if (tick && i % 5 === 0) {
							await tick();
						}
					}
					// Ensure final tick
					if (tick) await tick();
				} catch (error) {
					console.error("[Typewriter] Error", error);
				}
			})
			.catch((error) => {
				console.error("[Typewriter] Queue Error", error);
				return Promise.resolve();
			});
	};

	const cancel = () => {
		isCancelled = true;
	};

	const getPromise = () => queue;

	return { add, cancel, getPromise };
}

/**
 * Creates a streaming renderer that immediately updates state for each chunk
 * This leverages Svelte's reactivity system for incremental rendering
 * Supports optional typewriter effect for character-by-character rendering
 */
export function createStreamingRenderer(
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
) {
	const {
		onChunk,
		onComplete,
		tick,
		typewriter = false,
		typewriterDelay = 10,
	} = options;

	let chunkCount = 0;
	let totalLength = 0;
	let isCancelled = false;

	// Initialize typewriter effect if enabled
	const typewriterEffect = typewriter
		? simulateTypewriterEffect(updateState, { tick, delay: typewriterDelay })
		: null;

	/**
	 * Process a decoded text chunk
	 * Immediately updates state for each chunk to trigger Svelte reactivity
	 * Uses tick() if provided to ensure immediate re-render
	 * Supports typewriter effect for character-by-character rendering
	 */
	const processChunk = async (decoded: string): Promise<void> => {
		if (!decoded || isCancelled) return;

		try {
			chunkCount++;

			if (typewriterEffect) {
				// Render character by character with typewriter effect
				// Do not await/block here, let it run in background
				typewriterEffect.add(decoded);
			} else {
				// Immediately update state for each chunk to trigger Svelte reactivity
				updateState(decoded);
			}

			totalLength += decoded.length;

			// Log chunk if callback provided
			if (onChunk && chunkCount <= 5) {
				onChunk(
					chunkCount,
					decoded.length,
					decoded.length,
					totalLength,
					decoded.substring(0, 50),
				);
			}

			// If tick() is provided and not using typewriter, use it to force immediate re-render
			// This ensures Svelte processes the state update immediately
			if (!typewriter && tick) {
				await tick();
			} else if (!typewriter) {
				// Without tick(), rely on Svelte's automatic reactivity
				// Use setTimeout(0) to yield to the event loop and allow Svelte to process updates
				await new Promise<void>((resolve) => setTimeout(resolve, 0));
			}
		} catch (error) {
			console.error("[StreamingRenderer] Error processing chunk", error);
			// Don't re-throw to prevent breaking the stream, but log the error
		}
	};

	/**
	 * Flush any remaining data (call when stream completes)
	 * No buffering, so this is just for cleanup and callbacks
	 */
	const flush = () => {
		if (onComplete) {
			onComplete(totalLength, chunkCount);
		}
	};

	/**
	 * Cancel rendering (useful for cleanup when stream is interrupted)
	 */
	const cancel = () => {
		isCancelled = true;
		if (typewriterEffect) {
			typewriterEffect.cancel();
		}
	};

	/**
	 * Reset the renderer state
	 */
	const reset = () => {
		chunkCount = 0;
		totalLength = 0;
		isCancelled = false;
	};

	return {
		processChunk: processChunk as (decoded: string) => Promise<void>,
		flush,
		reset,
		cancel,
		getChunkCount: () => chunkCount,
		getTotalLength: () => totalLength,
	};
}

// Re-export SSE parser and processors for backward compatibility
export { parseSSEStream, type SSEEvent } from "./sse-parser";
export {
	processAugurStreamingText,
	processGenericStreamingText,
	processSummarizeStreamingText,
	processStreamingText,
} from "./sse-processors";
