import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock $lib/connect
const mockStreamAugurChat = vi.fn();
vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamAugurChat: (...args: unknown[]) => mockStreamAugurChat(...args),
}));

import { useAugurPane } from "./useAugurPane.svelte";

describe("useAugurPane", () => {
	let capturedCallbacks: {
		onDelta?: (text: string) => void;
		onThinking?: (text: string) => void;
		onMeta?: (
			citations: Array<{ url: string; title: string; publishedAt: string }>,
		) => void;
		onComplete?: (result: {
			answer: string;
			citations: Array<{ url: string; title: string; publishedAt: string }>;
		}) => void;
		onFallback?: (code: string) => void;
		onError?: (error: Error) => void;
		onProgress?: (stage: string) => void;
	};
	let mockAbortController: AbortController;

	beforeEach(() => {
		vi.clearAllMocks();
		capturedCallbacks = {};
		mockAbortController = new AbortController();

		mockStreamAugurChat.mockImplementation(
			(
				_transport: unknown,
				_options: unknown,
				onDelta?: (text: string) => void,
				onThinking?: (text: string) => void,
				onMeta?: (
					citations: Array<{ url: string; title: string; publishedAt: string }>,
				) => void,
				onComplete?: (result: {
					answer: string;
					citations: Array<{ url: string; title: string; publishedAt: string }>;
				}) => void,
				onFallback?: (code: string) => void,
				onError?: (error: Error) => void,
				onProgress?: (stage: string) => void,
			) => {
				capturedCallbacks = {
					onDelta,
					onThinking,
					onMeta,
					onComplete,
					onFallback,
					onError,
					onProgress,
				};
				return mockAbortController;
			},
		);
	});

	describe("initial state", () => {
		it("starts with empty messages and idle state", () => {
			const pane = useAugurPane();
			expect(pane.messages).toEqual([]);
			expect(pane.isLoading).toBe(false);
			expect(pane.progressStage).toBe("");
		});
	});

	describe("reset()", () => {
		it("clears messages and sets isLoading to false", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");
			expect(pane.messages.length).toBeGreaterThan(0);
			expect(pane.isLoading).toBe(true);

			pane.reset();
			expect(pane.messages).toEqual([]);
			expect(pane.isLoading).toBe(false);
			expect(pane.progressStage).toBe("");
		});

		it("aborts active stream", () => {
			const pane = useAugurPane();
			const abortSpy = vi.spyOn(mockAbortController, "abort");

			pane.sendMessage("Hello");
			pane.reset();

			expect(abortSpy).toHaveBeenCalled();
		});
	});

	describe("sendMessage()", () => {
		it("adds user message and assistant placeholder", () => {
			const pane = useAugurPane();
			pane.sendMessage("What is RSS?");

			expect(pane.messages).toHaveLength(2);
			expect(pane.messages[0].role).toBe("user");
			expect(pane.messages[0].message).toBe("What is RSS?");
			expect(pane.messages[1].role).toBe("assistant");
			expect(pane.messages[1].message).toBe("");
		});

		it("sets isLoading to true", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");
			expect(pane.isLoading).toBe(true);
		});

		it("calls streamAugurChat with correct message history", () => {
			const pane = useAugurPane();
			pane.sendMessage("What is RSS?");

			expect(mockStreamAugurChat).toHaveBeenCalledWith(
				expect.anything(), // transport
				{
					messages: [{ role: "user", content: "What is RSS?" }],
				},
				expect.any(Function), // onDelta
				expect.any(Function), // onThinking
				expect.any(Function), // onMeta
				expect.any(Function), // onComplete
				expect.any(Function), // onFallback
				expect.any(Function), // onError
				expect.any(Function), // onProgress
			);
		});

		it("aborts previous stream before starting new one", () => {
			const firstAbort = new AbortController();
			const secondAbort = new AbortController();
			const abortSpy = vi.spyOn(firstAbort, "abort");

			mockStreamAugurChat
				.mockReturnValueOnce(firstAbort)
				.mockReturnValueOnce(secondAbort);

			const pane = useAugurPane();
			pane.sendMessage("First question");
			pane.sendMessage("Second question");

			expect(abortSpy).toHaveBeenCalled();
		});

		it("builds correct history for multi-turn conversation", () => {
			const pane = useAugurPane();

			// First turn
			pane.sendMessage("What is RSS?");
			// Simulate completion
			capturedCallbacks.onComplete?.({
				answer: "RSS is a web feed format.",
				citations: [],
			});

			// Second turn
			pane.sendMessage("Tell me more");

			// The second call should include full history (excluding the empty placeholder)
			const secondCall = mockStreamAugurChat.mock.calls[1];
			const options = secondCall[1] as {
				messages: Array<{ role: string; content: string }>;
			};
			expect(options.messages).toEqual([
				{ role: "user", content: "What is RSS?" },
				{ role: "assistant", content: "RSS is a web feed format." },
				{ role: "user", content: "Tell me more" },
			]);
		});
	});

	describe("abort()", () => {
		it("calls AbortController.abort()", () => {
			const pane = useAugurPane();
			const abortSpy = vi.spyOn(mockAbortController, "abort");

			pane.sendMessage("Hello");
			pane.abort();

			expect(abortSpy).toHaveBeenCalled();
		});

		it("sets isLoading to false", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");
			expect(pane.isLoading).toBe(true);

			pane.abort();
			expect(pane.isLoading).toBe(false);
		});

		it("does nothing when no stream is active", () => {
			const pane = useAugurPane();
			expect(() => pane.abort()).not.toThrow();
		});
	});

	describe("streaming callbacks", () => {
		it("onDelta accumulates text in assistant message", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onDelta?.("RSS ");
			capturedCallbacks.onDelta?.("is ");
			capturedCallbacks.onDelta?.("great.");

			expect(pane.messages[1].message).toBe("RSS is great.");
		});

		it("onComplete finalizes assistant message and clears isLoading", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onDelta?.("partial");
			capturedCallbacks.onComplete?.({
				answer: "Final answer text",
				citations: [
					{
						url: "https://example.com",
						title: "Example",
						publishedAt: "2026-01-01",
					},
				],
			});

			expect(pane.messages[1].message).toBe("Final answer text");
			expect(pane.messages[1].citations).toEqual([
				{
					URL: "https://example.com",
					Title: "Example",
					PublishedAt: "2026-01-01",
				},
			]);
			expect(pane.isLoading).toBe(false);
			expect(pane.progressStage).toBe("");
		});

		it("onMeta updates citations on assistant message", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onMeta?.([
				{ url: "https://a.com", title: "A", publishedAt: "2026-01-01" },
			]);

			expect(pane.messages[1].citations).toEqual([
				{ URL: "https://a.com", Title: "A", PublishedAt: "2026-01-01" },
			]);
		});

		it("onError sets error message in assistant message", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onError?.(new Error("Network failure"));

			expect(pane.messages[1].message).toContain("Network failure");
			expect(pane.isLoading).toBe(false);
		});

		it("onFallback sets fallback message for insufficient context", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onFallback?.("insufficient_context");

			expect(pane.messages[1].message).toContain("Not enough indexed evidence");
			expect(pane.isLoading).toBe(false);
		});

		it("onProgress updates progressStage", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onProgress?.("searching");
			expect(pane.progressStage).toBe("searching");

			capturedCallbacks.onProgress?.("generating");
			expect(pane.progressStage).toBe("generating");
		});
	});

	describe("timeout", () => {
		it("auto-recovers after 180 seconds if onComplete never fires", () => {
			vi.useFakeTimers();
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			// Simulate partial delta but no onComplete
			capturedCallbacks.onDelta?.("Partial text");
			expect(pane.isLoading).toBe(true);

			// Advance 180 seconds
			vi.advanceTimersByTime(180_000);

			expect(pane.isLoading).toBe(false);
			expect(pane.messages[1].message).toContain("Partial text");

			vi.useRealTimers();
		});

		it("timeout is cleared when onComplete fires normally", () => {
			vi.useFakeTimers();
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onComplete?.({ answer: "Done", citations: [] });
			expect(pane.isLoading).toBe(false);

			// Advance past timeout — should NOT change anything
			vi.advanceTimersByTime(180_000);
			expect(pane.messages[1].message).toBe("Done");

			vi.useRealTimers();
		});
	});

	describe("provisional state", () => {
		it("onDelta sets isProvisional to true", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");
			expect(pane.isProvisional).toBe(false);

			capturedCallbacks.onDelta?.("Draft text");
			expect(pane.isProvisional).toBe(true);
		});

		it("onComplete clears isProvisional", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onDelta?.("Draft");
			expect(pane.isProvisional).toBe(true);

			capturedCallbacks.onComplete?.({
				answer: "Final answer",
				citations: [],
			});
			expect(pane.isProvisional).toBe(false);
		});

		it("onFallback clears isProvisional", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onDelta?.("Draft");
			expect(pane.isProvisional).toBe(true);

			capturedCallbacks.onFallback?.("insufficient_context");
			expect(pane.isProvisional).toBe(false);
		});
	});

	describe("statusText from thinking", () => {
		it("onThinking updates statusText", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onThinking?.("Analyzing context...");
			expect(pane.statusText).toBe("Analyzing context...");
		});

		it("refining progress updates statusText", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onProgress?.("refining");
			expect(pane.progressStage).toBe("refining");
			expect(pane.statusText).toBe("Refining answer...");
		});

		it("onComplete clears statusText", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onThinking?.("Thinking...");
			capturedCallbacks.onComplete?.({ answer: "Done", citations: [] });
			expect(pane.statusText).toBe("");
		});
	});

	describe("fallback messages", () => {
		it("shows article-specific message for insufficient context", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onFallback?.(
				"retrieval quality insufficient: context relevance too low",
			);

			expect(pane.messages[1].message).toContain("Not enough indexed evidence");
		});

		it("shows generic message for technical fallback codes", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onFallback?.("validation failed");

			expect(pane.messages[1].message).toContain(
				"I couldn't find enough information",
			);
		});

		it("shows Japanese fallback reasons as-is", () => {
			const pane = useAugurPane();
			pane.sendMessage("Hello");

			capturedCallbacks.onFallback?.(
				"十分に一貫した根拠が取れなかったため、因果関係を断定できません。より具体的な質問をお試しください。",
			);

			expect(pane.messages[1].message).toContain(
				"I couldn't establish a consistent enough evidence trail",
			);
		});
	});
});
