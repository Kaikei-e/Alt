/**
 * The Ask Augur consultation, at both the widths it is read at.
 *
 * These cover what the phone-only `ChatWindow` used to assert on its own, plus
 * the promises that only became statable once there was a single component:
 * one question is asked once, and the stream stops when the component does.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

type StreamCall = {
	options: {
		messages: { role: string; content: string }[];
		conversationId?: string;
	};
	controller: AbortController;
	onComplete?: (result: {
		answer: string;
		citations: never[];
		relatedCitations: never[];
	}) => void;
	onError?: (error: Error) => void;
	onConversationId?: (id: string) => void;
};

const { streamCalls, streamAugurChat } = vi.hoisted(() => {
	const streamCalls: unknown[] = [];
	return {
		streamCalls,
		streamAugurChat: vi.fn(
			(
				_transport: unknown,
				options: unknown,
				_onDelta?: unknown,
				_onThinking?: unknown,
				_onMeta?: unknown,
				onComplete?: unknown,
				_onFallback?: unknown,
				onError?: unknown,
				_onProgress?: unknown,
				onConversationId?: unknown,
			) => {
				const controller = new AbortController();
				streamCalls.push({
					options,
					controller,
					onComplete,
					onError,
					onConversationId,
				});
				return controller;
			},
		),
	};
});

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamAugurChat,
}));

import AugurChat from "./AugurChat.svelte";

const PHONE = { width: 393, height: 851 };
const DESKTOP = { width: 1280, height: 900 };

const calls = () => streamCalls as StreamCall[];
const input = () => page.getByPlaceholder("What would you like to know?");
const submit = () => page.getByRole("button", { name: /submit/i });

async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe.each([
	["a phone", PHONE],
	["a desktop", DESKTOP],
])("AugurChat on %s", (_name, viewport) => {
	beforeEach(async () => {
		streamCalls.length = 0;
		streamAugurChat.mockClear();
		await page.viewport(viewport.width, viewport.height);
	});

	it("opens on the invitation with somewhere to type", async () => {
		render(AugurChat);

		await expect.element(page.getByText("Ask Augur")).toBeInTheDocument();
		await expect.element(input()).toBeInTheDocument();
	});

	it("sends what was typed and shows it in the thread", async () => {
		render(AugurChat);

		await input().fill("Hello Augur");
		await submit().click();

		await expect.element(input()).toHaveValue("");
		await expect.element(page.getByText("Hello Augur")).toBeInTheDocument();
		expect(calls()).toHaveLength(1);
	});

	it("asks the question the URL arrived with, exactly once", async () => {
		render(AugurChat, {
			props: {
				initialQuestion:
					"Context:\nAI chip summary\n\nQuestion:\nWhat changed?",
			},
		});

		await expect.element(page.getByText("What changed?")).toBeInTheDocument();
		await settle();
		expect(calls()).toHaveLength(1);
	});

	it("hydrates a stored conversation instead of the empty state", async () => {
		render(AugurChat, {
			props: {
				initialMessages: [
					{
						id: "user-0",
						role: "user" as const,
						message: "Tell me about quantum chips",
						timestamp: "",
					},
					{
						id: "assistant-1",
						role: "assistant" as const,
						message: "Quantum chips use qubits to perform computations.",
						timestamp: "",
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

	it("threads a follow-up onto the conversation it was given", async () => {
		render(AugurChat, {
			props: {
				initialMessages: [
					{
						id: "user-0",
						role: "user" as const,
						message: "first",
						timestamp: "",
					},
				],
				initialConversationId: "conv-abc-123",
			},
		});

		await input().fill("follow-up");
		await submit().click();

		expect(calls()[0]?.options.conversationId).toBe("conv-abc-123");
	});

	it("hands the persisted conversation id back to the page", async () => {
		const onConversationIdChange = vi.fn();
		render(AugurChat, { props: { onConversationIdChange } });

		await input().fill("What changed today?");
		await submit().click();

		calls()[0]?.onConversationId?.("conv-42");

		expect(onConversationIdChange).toHaveBeenCalledWith("conv-42");
	});

	it("does not send on Enter while an IME is composing", async () => {
		render(AugurChat);

		await input().fill("こんにちは");
		const textarea =
			document.querySelector<HTMLTextAreaElement>("#augur-input");
		expect(textarea).not.toBeNull();
		textarea?.dispatchEvent(
			new CompositionEvent("compositionstart", { bubbles: true }),
		);
		const enter = new KeyboardEvent("keydown", {
			key: "Enter",
			bubbles: true,
			cancelable: true,
		});
		Object.defineProperty(enter, "isComposing", { value: true });
		textarea?.dispatchEvent(enter);
		await settle();

		expect(calls()).toHaveLength(0);
		await expect.element(input()).toHaveValue("こんにちは");
	});

	it("closes the input while the answer is being written", async () => {
		render(AugurChat);

		await input().fill("Test question");
		await submit().click();

		await expect.element(input()).toBeDisabled();
	});

	it("says what went wrong when the stream fails", async () => {
		render(AugurChat);

		await input().fill("Test question");
		await submit().click();

		calls()[0]?.onError?.(new Error("upstream unavailable"));

		await expect
			.element(page.getByText(/upstream unavailable/))
			.toBeInTheDocument();
	});

	// The stream is billed for as long as it runs. Nothing keeps paying for an
	// answer with nowhere to land.
	it("stops the stream when it is destroyed", async () => {
		const { unmount } = render(AugurChat);

		await input().fill("Test question");
		await submit().click();
		expect(calls()).toHaveLength(1);

		await unmount();

		expect(calls()[0]?.controller.signal.aborted).toBe(true);
	});
});
