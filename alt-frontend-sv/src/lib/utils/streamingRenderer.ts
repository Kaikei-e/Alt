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

/**
 * Generic SSE Event
 */
export interface SSEEvent {
	id?: string;
	event: string;
	data: string;
	retry?: number;
}

/**
 * Parses a readable stream into SSE events
 */
export async function* parseSSEStream(
	reader: ReadableStreamDefaultReader<Uint8Array>,
): AsyncGenerator<SSEEvent> {
	const decoder = new TextDecoder("utf-8");
	let buffer = "";
	let currentEvent: SSEEvent = { event: "message", data: "" };
	let hasData = false;

	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) {
				// Process last bits if any
				if (buffer.trim()) {
					const lines = buffer.split("\n");
					for (const line of lines) {
						const trimmed = line.trim();
						// Simple logical check for data lines in leftover buffer
						if (trimmed.startsWith("data:")) {
							let content = line.substring(line.indexOf(":") + 1);
							if (content.startsWith(" ")) content = content.substring(1);
							currentEvent.data += content + "\n";
							hasData = true;
						}
					}
					if (hasData) {
						const data = currentEvent.data.endsWith("\n")
							? currentEvent.data.slice(0, -1)
							: currentEvent.data;
						yield { ...currentEvent, data };
					}
				}
				break;
			}

			if (value) {
				const chunk = decoder.decode(value, { stream: true });
				buffer += chunk;

				let boundary = buffer.indexOf("\n");
				while (boundary !== -1) {
					const line = buffer.slice(0, boundary);
					buffer = buffer.slice(boundary + 1);

					const trimmed = line.trim();
					if (!trimmed) {
						// End of event
						if (hasData) {
							const data = currentEvent.data.endsWith("\n")
								? currentEvent.data.slice(0, -1)
								: currentEvent.data;
							yield { ...currentEvent, data };
						}
						currentEvent = { event: "message", data: "" };
						hasData = false;
					} else if (trimmed.startsWith("event:")) {
						currentEvent.event = trimmed.slice(6).trim();
					} else if (trimmed.startsWith("data:")) {
						let content = line.substring(line.indexOf(":") + 1);
						if (content.startsWith(" ")) content = content.substring(1);
						currentEvent.data += content + "\n";
						hasData = true;
					} else if (trimmed.startsWith("id:")) {
						currentEvent.id = trimmed.slice(3).trim();
					} else if (trimmed.startsWith(":")) {
						// Comment
					}

					boundary = buffer.indexOf("\n");
				}
			}
		}
	} finally {
		try {
			reader.releaseLock();
		} catch (e) {}
	}
}

/**
 * Augur-specific streaming processor
 * Handles 'delta', 'meta', 'fallback' events
 */
export async function processAugurStreamingText(
	reader: ReadableStreamDefaultReader<Uint8Array>,
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
): Promise<{
	chunkCount: number;
	totalLength: number;
	hasReceivedData: boolean;
}> {
	const renderer = createStreamingRenderer(updateState, options);
	let hasReceivedData = false;

	try {
		for await (const event of parseSSEStream(reader)) {
			if (event.event === "delta") {
				if (event.data) {
					await renderer.processChunk(event.data);
					hasReceivedData = true;
				}
			} else if (event.event === "meta" || event.event === "done") {
				if (options.onMetadata && event.data) {
					try {
						const parsed = JSON.parse(event.data);
						options.onMetadata(parsed);
					} catch (e) {
						console.warn("[AugurStream] Failed to parse metadata", e);
					}
				}
			} else if (event.event === "fallback") {
				if (options.onMetadata) {
					options.onMetadata({ fallback: true, code: event.data });
				}
			} else if (event.event === "error") {
				console.error("[AugurStream] Error Event:", event.data);
			}
		}
		renderer.flush();
	} catch (error) {
		renderer.cancel();
		throw error;
	}

	return {
		chunkCount: renderer.getChunkCount(),
		totalLength: renderer.getTotalLength(),
		hasReceivedData,
	};
}

/**
 * Generic Streaming Text Processor
 * Treats everything as text content unless specific event handling is added.
 * Useful for standard text-only streams.
 */
export async function processGenericStreamingText(
	reader: ReadableStreamDefaultReader<Uint8Array>,
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
): Promise<void> {
	const renderer = createStreamingRenderer(updateState, options);
	try {
		for await (const event of parseSSEStream(reader)) {
			// Treat all events data as content for now, or just 'message'/'delta'
			if (event.data) {
				await renderer.processChunk(event.data);
			}
		}
		renderer.flush();
	} catch (e) {
		renderer.cancel();
		throw e;
	}
}

/**
 * Summarize-specific streaming processor (News Creator)
 * Handles standard SSE 'message' events where data is a JSON string of the text chunk.
 */
export async function processSummarizeStreamingText(
	reader: ReadableStreamDefaultReader<Uint8Array>,
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
): Promise<{
	chunkCount: number;
	totalLength: number;
	hasReceivedData: boolean;
}> {
	const renderer = createStreamingRenderer(updateState, options);

	try {
		for await (const event of parseSSEStream(reader)) {
			if (event.data) {
				// Handling "message" (default) or "delta".
				if (event.event === "message" || event.event === "delta") {
					try {
						// Parse JSON string (e.g. '"Hello"') -> 'Hello'
						const text = JSON.parse(event.data);
						if (typeof text === "string") {
							await renderer.processChunk(text);
						} else if (text && typeof text === "object") {
							// If it's a complex object (rare for this usecase), stringify it
							await renderer.processChunk(JSON.stringify(text));
						} else {
							// Numbers, booleans, etc
							await renderer.processChunk(String(text));
						}
					} catch (e) {
						// Not valid JSON, treat as raw text (fallback)
						await renderer.processChunk(event.data);
					}
				}
			}
		}
		renderer.flush();
	} catch (error) {
		renderer.cancel();
		throw error;
	}

	return {
		chunkCount: renderer.getChunkCount(),
		totalLength: renderer.getTotalLength(),
		hasReceivedData: renderer.getChunkCount() > 0,
	};
}

// @deprecated Use processAugurStreamingText or processSummarizeStreamingText
export const processStreamingText = processAugurStreamingText;
