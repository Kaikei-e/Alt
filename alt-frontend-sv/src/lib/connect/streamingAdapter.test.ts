import type { Transport } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { StreamSummarizeChunk, StreamSummarizeResult } from "./feeds";
import { streamSummarizeWithAbortAdapter } from "./streamingAdapter";

vi.mock("./feeds", () => ({
	streamSummarizeWithAbort: vi.fn(),
}));

const { streamSummarizeWithAbort } = await import("./feeds");

interface CapturedCallbacks {
	onChunk: (chunk: StreamSummarizeChunk) => Promise<void> | void;
	onComplete: (result: StreamSummarizeResult) => void;
	onError: (error: Error) => void;
}

let captured: CapturedCallbacks;
let streamController: AbortController;

const transport = {} as Transport;

function textChunk(text: string): StreamSummarizeChunk {
	return {
		chunk: text,
		isFinal: false,
		articleId: "article-1",
		isCached: false,
		fullSummary: null,
	};
}

beforeEach(() => {
	vi.useFakeTimers();
	streamController = new AbortController();
	vi.mocked(streamSummarizeWithAbort).mockImplementation(
		(_transport, _options, onChunk, onComplete, onError) => {
			captured = {
				onChunk,
				onComplete,
				onError: onError ?? (() => {}),
			};
			return streamController;
		},
	);
});

afterEach(() => {
	vi.useRealTimers();
	vi.clearAllMocks();
});

describe("streamSummarizeWithAbortAdapter on a cut stream", () => {
	test("flushes every received character before onError fires", async () => {
		const received = "The quick brown fox jumps over the lazy dog";
		let rendered = "";
		let renderedWhenErrorSurfaced: string | null = null;

		streamSummarizeWithAbortAdapter(
			transport,
			{ feedUrl: "https://example.com/article" },
			(text) => {
				rendered += text;
			},
			{ typewriter: true, typewriterDelay: 10 },
			undefined,
			() => {
				renderedWhenErrorSurfaced = rendered;
			},
		);

		await captured.onChunk(textChunk(received));
		// The typewriter ceiling is slower than arrival: only a few characters
		// have reached the UI when the stream is cut.
		await vi.advanceTimersByTimeAsync(20);
		expect(rendered.length).toBeLessThan(received.length);

		captured.onError(new Error("missing EndStreamResponse"));

		expect(renderedWhenErrorSurfaced).toBe(received);
	});

	test("keeps the flushed text after the error, without re-typing it", async () => {
		const received = "Partial summary text that never finished";
		let rendered = "";

		streamSummarizeWithAbortAdapter(
			transport,
			{ feedUrl: "https://example.com/article" },
			(text) => {
				rendered += text;
			},
			{ typewriter: true, typewriterDelay: 10 },
		);

		await captured.onChunk(textChunk(received));
		await vi.advanceTimersByTimeAsync(20);
		captured.onError(new Error("missing EndStreamResponse"));

		await vi.advanceTimersByTimeAsync(5000);

		expect(rendered).toBe(received);
	});

	test("still drops the backlog when the reader aborted locally", async () => {
		const received = "Text the reader chose not to wait for";
		let rendered = "";

		streamSummarizeWithAbortAdapter(
			transport,
			{ feedUrl: "https://example.com/article" },
			(text) => {
				rendered += text;
			},
			{ typewriter: true, typewriterDelay: 10 },
		);

		await captured.onChunk(textChunk(received));
		await vi.advanceTimersByTimeAsync(20);

		streamController.abort();
		captured.onError(new Error("The operation was aborted"));
		await vi.advanceTimersByTimeAsync(5000);

		expect(rendered.length).toBeLessThan(received.length);
	});

	test("still drops the backlog on an AbortError without an aborted signal", async () => {
		const received = "Text the reader chose not to wait for";
		let rendered = "";

		streamSummarizeWithAbortAdapter(
			transport,
			{ feedUrl: "https://example.com/article" },
			(text) => {
				rendered += text;
			},
			{ typewriter: true, typewriterDelay: 10 },
		);

		await captured.onChunk(textChunk(received));
		await vi.advanceTimersByTimeAsync(20);

		const abortError = new Error("The operation was aborted");
		abortError.name = "AbortError";
		captured.onError(abortError);
		await vi.advanceTimersByTimeAsync(5000);

		expect(rendered.length).toBeLessThan(received.length);
	});

	test("surfaces the error to the caller either way", async () => {
		const onError = vi.fn();

		streamSummarizeWithAbortAdapter(
			transport,
			{ feedUrl: "https://example.com/article" },
			() => {},
			{ typewriter: true, typewriterDelay: 10 },
			undefined,
			onError,
		);

		const error = new Error("missing EndStreamResponse");
		captured.onError(error);

		expect(onError).toHaveBeenCalledWith(error);
	});
});
