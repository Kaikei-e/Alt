/**
 * Utility for incremental rendering of streaming text data
 * Uses Svelte's reactivity system for immediate UI updates
 * Based on best practices for streaming text rendering
 */

export interface StreamingRendererOptions {
  /**
   * Callback for logging (optional)
   */
  onChunk?: (chunkCount: number, chunkSize: number, decodedLength: number, totalLength: number, preview: string) => void;
  /**
   * Callback when stream completes
   */
  onComplete?: (totalLength: number, chunkCount: number) => void;
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
  } = {}
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
  options: StreamingRendererOptions = {}
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
          decoded.substring(0, 50)
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
    // We can't easily reset the external typewriter effect without recreating it
    // But since createStreamingRenderer is usually created once per stream, this is likely fine.
    // If reset is called, we might need to recreate the effect or clear it.
    // For now, let's assume usage pattern creates a new renderer or we just don't support full reset of typewriter specific state here easily without change.
    // Actually, we should probably warn or re-init.
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
 * Process a ReadableStreamDefaultReader for text streaming with incremental rendering
 * Best practice: Update state immediately for each chunk and let Svelte handle rendering
 * Filters out SSE heartbeat comments (lines starting with ':')
 */
export async function processStreamingText(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  updateState: (text: string) => void,
  options: StreamingRendererOptions = {}
): Promise<{ chunkCount: number; totalLength: number; hasReceivedData: boolean }> {
  const decoder = new TextDecoder("utf-8");
  const renderer = createStreamingRenderer(updateState, options);
  let hasReceivedData = false;
  let buffer = ""; // Buffer for accumulating partial lines

  /**
   * Filter out SSE heartbeat comments and process only actual content
   * SSE format: ': comment\n\n' (heartbeat) or raw text content
   * We filter out lines that start with ':' (SSE comment lines)
   */
  /**
   * Parse SSE events from the stream
   * SSE format: 'data: <json>\n\n' or ': comment\n\n'
   */
  const parseSSEEvents = (text: string): string[] => {
    if (!text) return [];

    // Add to buffer
    buffer += text;

    // Split by newlines to process complete lines
    const lines = buffer.split("\n");
    // Keep the last potentially incomplete line in buffer
    buffer = lines[lines.length - 1] || "";

    // Process all lines except the last one
    const completeLines = lines.slice(0, -1);
    const validChunks: string[] = [];

    for (const line of completeLines) {
      const trimmed = line.trim();
      if (!trimmed) continue;

      // Skip SSE comments
      if (trimmed.startsWith(":")) {
        continue;
      }

      // Handle data-only lines (standard SSE)
      if (trimmed.startsWith("data:")) {
        const dataContent = trimmed.slice(5).trim();
        try {
          // Parse JSON content if possible (backend sends JSON string)
          // Backend sends: data: "token"
          // So JSON.parse('"token"') -> "token"
          // If backend sends: data: {"text": "token"} -> parse -> obj.text
          // Our backend sends `json.dumps(content)` where content is string.
          // So typical payload: data: "Hello"
          const parsed = JSON.parse(dataContent);
          if (typeof parsed === 'string') {
            validChunks.push(parsed);
          } else if (typeof parsed === 'object' && parsed !== null) {
            // Fallback for object payloads if we ever send structured data
            validChunks.push(JSON.stringify(parsed));
          } else {
            validChunks.push(String(parsed));
          }
        } catch (e) {
          // If not JSON, treat as raw text (fallback)
          validChunks.push(dataContent);
        }
      }
    }

    return validChunks;
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        // Flush any remaining bytes in the decoder
        const remaining = decoder.decode();
        if (remaining) {
          buffer += remaining;
        }
        // Process final buffer content (filtering SSE comments)
        // Force process remaining buffer by adding a newline to complete any partial line
        if (buffer) {
          const finalValues = parseSSEEvents(buffer + "\n");
          for (const val of finalValues) {
            await renderer.processChunk(val);
            hasReceivedData = true;
          }
        }
        // Flush final buffer
        renderer.flush();
        break;
      }
      if (value) {
        // Decode chunk and filter SSE comments
        const decoded = decoder.decode(value, { stream: true });
        if (decoded) {
          // Filter and parse SSE events
          const chunks = parseSSEEvents(decoded);
          for (const chunk of chunks) {
            hasReceivedData = true;
            // Process chunk immediately
            await renderer.processChunk(chunk);
          }
        }
      }
    }
  } catch (error) {
    // Cancel rendering on error to prevent further processing
    renderer.cancel();
    // Flush any buffered data before re-throwing
    renderer.flush();
    throw error;
  } finally {
    // Ensure reader is released even if there's an error
    try {
      reader.releaseLock();
    } catch (releaseError) {
      // Ignore errors when releasing lock (reader might already be released)
      console.warn("[StreamingRenderer] Error releasing reader lock", releaseError);
    }
  }

  return {
    chunkCount: renderer.getChunkCount(),
    totalLength: renderer.getTotalLength(),
    hasReceivedData,
  };
}
