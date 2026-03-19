import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithAbortAdapter: vi.fn(),
}));

vi.mock("$lib/utils/errorClassification", () => ({
	isTransientError: vi.fn(() => false),
}));

import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import { isTransientError } from "$lib/utils/errorClassification";
import { useSummarize } from "./useSummarize.svelte";

/** Helper: capture the callbacks passed to streamSummarizeWithAbortAdapter */
function captureAdapterCallbacks() {
	const calls = vi.mocked(streamSummarizeWithAbortAdapter).mock.calls;
	const lastCall = calls[calls.length - 1];
	return {
		transport: lastCall[0],
		options: lastCall[1],
		updateState: lastCall[2] as (text: string) => void,
		rendererOptions: lastCall[3],
		onComplete: lastCall[4] as (result: unknown) => void,
		onError: lastCall[5] as (error: Error) => void,
	};
}

describe("useSummarize", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(streamSummarizeWithAbortAdapter).mockReturnValue(
			new AbortController(),
		);
	});

	describe("initial state", () => {
		it("starts in idle state", () => {
			const s = useSummarize();
			expect(s.summary).toBeNull();
			expect(s.isSummarizing).toBe(false);
			expect(s.summaryError).toBeNull();
			expect(s.buttonState).toBe("idle");
		});
	});

	describe("buttonState derivation", () => {
		it("returns loading when summarizing", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			expect(s.buttonState).toBe("loading");
		});

		it("returns success when summary exists", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { updateState, onComplete } = captureAdapterCallbacks();
			updateState("chunk1");
			onComplete({ articleId: "a1", wasCached: false });

			expect(s.buttonState).toBe("success");
		});

		it("returns error when summaryError exists", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { onError } = captureAdapterCallbacks();
			onError(new Error("Server error"));

			expect(s.buttonState).toBe("error");
		});
	});

	describe("summarize()", () => {
		it("calls streamSummarizeWithAbortAdapter with correct params", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed", "art-1", "My Title");

			expect(createClientTransport).toHaveBeenCalled();
			expect(streamSummarizeWithAbortAdapter).toHaveBeenCalledWith(
				expect.anything(), // transport
				{
					feedUrl: "https://example.com/feed",
					articleId: "art-1",
					title: "My Title",
					forceRefresh: false,
				},
				expect.any(Function), // updateState
				{}, // rendererOptions
				expect.any(Function), // onComplete
				expect.any(Function), // onError
			);
		});

		it("passes forceRefresh when specified", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed", undefined, undefined, true);

			expect(streamSummarizeWithAbortAdapter).toHaveBeenCalledWith(
				expect.anything(),
				expect.objectContaining({ forceRefresh: true }),
				expect.any(Function),
				{},
				expect.any(Function),
				expect.any(Function),
			);
		});

		it("sets isSummarizing to true", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			expect(s.isSummarizing).toBe(true);
		});

		it("does not call adapter when already summarizing", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");
			s.summarize("https://example.com/feed");

			// First call aborts, second call proceeds
			expect(streamSummarizeWithAbortAdapter).toHaveBeenCalledTimes(2);
		});
	});

	describe("chunk accumulation", () => {
		it("accumulates chunks into summary", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { updateState } = captureAdapterCallbacks();
			updateState("Hello ");
			expect(s.summary).toBe("Hello ");

			updateState("Hello World");
			expect(s.summary).toBe("Hello Hello World");
		});
	});

	describe("completion", () => {
		it("transitions to success on complete", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { updateState, onComplete } = captureAdapterCallbacks();
			updateState("Summary text");
			onComplete({ articleId: "a1", wasCached: false });

			expect(s.isSummarizing).toBe(false);
			expect(s.summary).toBe("Summary text");
			expect(s.summaryError).toBeNull();
			expect(s.buttonState).toBe("success");
		});
	});

	describe("error handling", () => {
		it("transitions to error on failure", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { onError } = captureAdapterCallbacks();
			onError(new Error("API Error"));

			expect(s.isSummarizing).toBe(false);
			expect(s.summaryError).toBe("API Error");
			expect(s.buttonState).toBe("error");
		});

		it("ignores AbortError", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { onError } = captureAdapterCallbacks();
			const abortErr = new Error("AbortError");
			abortErr.name = "AbortError";
			onError(abortErr);

			expect(s.isSummarizing).toBe(false);
			expect(s.summaryError).toBeNull();
			expect(s.buttonState).toBe("idle");
		});

		it("ignores errors with abort in message", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { onError } = captureAdapterCallbacks();
			onError(new Error("The request was aborted"));

			expect(s.summaryError).toBeNull();
		});

		it("ignores errors with cancel in message", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { onError } = captureAdapterCallbacks();
			onError(new Error("Operation was cancelled"));

			expect(s.summaryError).toBeNull();
		});

		it("retries once on transient error", () => {
			vi.mocked(isTransientError).mockReturnValue(true);
			vi.useFakeTimers();

			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { onError } = captureAdapterCallbacks();
			onError(new Error("503 Service Unavailable"));

			// After timer, should retry
			vi.advanceTimersByTime(500);

			expect(streamSummarizeWithAbortAdapter).toHaveBeenCalledTimes(2);

			vi.useRealTimers();
		});

		it("does not retry more than once on transient errors", () => {
			vi.mocked(isTransientError).mockReturnValue(true);
			vi.useFakeTimers();

			const s = useSummarize();
			s.summarize("https://example.com/feed");

			// First error → triggers retry
			const cb1 = captureAdapterCallbacks();
			cb1.onError(new Error("503 Service Unavailable"));
			vi.advanceTimersByTime(500);

			// Second error → no more retries
			const cb2 = captureAdapterCallbacks();
			cb2.onError(new Error("503 Service Unavailable"));
			vi.advanceTimersByTime(500);

			expect(streamSummarizeWithAbortAdapter).toHaveBeenCalledTimes(2);
			expect(s.summaryError).toBe("503 Service Unavailable");

			vi.useRealTimers();
		});
	});

	describe("abort()", () => {
		it("aborts the active controller", () => {
			const mockController = new AbortController();
			const abortSpy = vi.spyOn(mockController, "abort");
			vi.mocked(streamSummarizeWithAbortAdapter).mockReturnValue(
				mockController,
			);

			const s = useSummarize();
			s.summarize("https://example.com/feed");
			s.abort();

			expect(abortSpy).toHaveBeenCalled();
		});

		it("does nothing when no active request", () => {
			const s = useSummarize();
			expect(() => s.abort()).not.toThrow();
		});
	});

	describe("reset()", () => {
		it("clears all state", () => {
			const s = useSummarize();
			s.summarize("https://example.com/feed");

			const { updateState, onComplete } = captureAdapterCallbacks();
			updateState("Some summary");
			onComplete({ articleId: "a1", wasCached: false });

			expect(s.summary).toBe("Some summary");

			s.reset();

			expect(s.summary).toBeNull();
			expect(s.isSummarizing).toBe(false);
			expect(s.summaryError).toBeNull();
			expect(s.buttonState).toBe("idle");
		});

		it("aborts any in-flight request", () => {
			const mockController = new AbortController();
			const abortSpy = vi.spyOn(mockController, "abort");
			vi.mocked(streamSummarizeWithAbortAdapter).mockReturnValue(
				mockController,
			);

			const s = useSummarize();
			s.summarize("https://example.com/feed");
			s.reset();

			expect(abortSpy).toHaveBeenCalled();
			expect(s.isSummarizing).toBe(false);
		});
	});
});
