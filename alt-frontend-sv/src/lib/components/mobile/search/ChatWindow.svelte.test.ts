import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ChatWindow from "./ChatWindow.svelte";

// Track typewriter calls
const mockAdd = vi.fn();
const mockCancel = vi.fn();

vi.mock("$lib/utils/streamingRenderer", () => ({
	simulateTypewriterEffect: vi.fn(() => ({
		add: mockAdd,
		cancel: mockCancel,
		getPromise: () => Promise.resolve(),
	})),
}));

// Capture streamAugurChat callbacks for manual invocation
let capturedOnDelta: ((text: string) => void) | undefined;
let capturedOnComplete:
	| ((result: { answer: string; citations: never[] }) => void)
	| undefined;
let capturedOptions: { messages: unknown; conversationId?: string } | undefined;

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamAugurChat: vi.fn(
		(
			_transport: unknown,
			options: { messages: unknown; conversationId?: string },
			onDelta?: (text: string) => void,
			_onThinking?: unknown,
			_onMeta?: unknown,
			onComplete?: (result: { answer: string; citations: never[] }) => void,
		) => {
			capturedOptions = options;
			capturedOnDelta = onDelta;
			capturedOnComplete = onComplete;
			return new AbortController();
		},
	),
}));

describe("ChatWindow", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		capturedOnDelta = undefined;
		capturedOnComplete = undefined;
		vi.stubGlobal("fetch", vi.fn());
	});

	it("renders correctly", async () => {
		render(ChatWindow);
		await expect
			.element(page.getByPlaceholder("What would you like to know?"))
			.toBeInTheDocument();
	});

	it("sends a message and displays user message", async () => {
		render(ChatWindow);
		const input = page.getByPlaceholder("What would you like to know?");
		const button = page.getByRole("button", { name: /submit/i });

		await input.fill("Hello Augur");
		await button.click();

		await expect.element(input).toHaveValue("");
		await expect.element(page.getByText("Hello Augur")).toBeInTheDocument();
	});

	it("auto-sends the initial question when provided", async () => {
		render(ChatWindow as never, {
			props: {
				initialQuestion:
					"Context:\nAI chip summary\n\nQuestion:\nWhat changed?",
			},
		});

		await expect.element(page.getByText("What changed?")).toBeInTheDocument();
	});

	it("hydrates initialMessages so prior conversation renders instead of empty state", async () => {
		render(ChatWindow as never, {
			props: {
				initialMessages: [
					{ role: "user", content: "Tell me about quantum chips" },
					{
						role: "assistant",
						content: "Quantum chips use qubits to perform computations.",
					},
				],
			},
		});

		await expect
			.element(page.getByText("Tell me about quantum chips"))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText("Quantum chips use qubits to perform computations."),
			)
			.toBeInTheDocument();
	});

	it("forwards initialConversationId to streamAugurChat so replies thread to the same conversation", async () => {
		render(ChatWindow as never, {
			props: {
				initialMessages: [{ role: "user", content: "first" }],
				initialConversationId: "conv-abc-123",
			},
		});

		const input = page.getByPlaceholder("What would you like to know?");
		const button = page.getByRole("button", { name: /submit/i });

		await input.fill("follow-up");
		await button.click();

		expect(capturedOptions).toBeDefined();
		expect(capturedOptions?.conversationId).toBe("conv-abc-123");
	});
});

describe("ChatWindow typewriter streaming", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		capturedOnDelta = undefined;
		capturedOnComplete = undefined;
		vi.stubGlobal("fetch", vi.fn());
	});

	it("uses simulateTypewriterEffect when streaming begins", async () => {
		const { simulateTypewriterEffect } = await import(
			"$lib/utils/streamingRenderer"
		);
		render(ChatWindow);

		const input = page.getByPlaceholder("What would you like to know?");
		const button = page.getByRole("button", { name: /submit/i });

		await input.fill("Test question");
		await button.click();

		expect(simulateTypewriterEffect).toHaveBeenCalled();
	});

	it("feeds delta text to typewriter.add instead of direct state update", async () => {
		render(ChatWindow);

		const input = page.getByPlaceholder("What would you like to know?");
		const button = page.getByRole("button", { name: /submit/i });

		await input.fill("Test question");
		await button.click();

		// Simulate a delta arriving from the stream
		expect(capturedOnDelta).toBeDefined();
		capturedOnDelta!("Hello ");
		capturedOnDelta!("world");

		expect(mockAdd).toHaveBeenCalledWith("Hello ");
		expect(mockAdd).toHaveBeenCalledWith("world");
	});

	it("cancels typewriter on stream completion", async () => {
		render(ChatWindow);

		const input = page.getByPlaceholder("What would you like to know?");
		const button = page.getByRole("button", { name: /submit/i });

		await input.fill("Test question");
		await button.click();

		expect(capturedOnComplete).toBeDefined();
		capturedOnComplete!({ answer: "Full answer", citations: [] });

		expect(mockCancel).toHaveBeenCalled();
	});
});
