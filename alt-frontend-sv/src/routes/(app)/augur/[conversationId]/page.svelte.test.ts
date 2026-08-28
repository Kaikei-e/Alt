/**
 * A conversation loaded from the server is the reader's own past. Turning the
 * phone must not throw it away — which it did, because the page mounted one of
 * two separately written chats depending on the width, and the loaded messages
 * lived inside whichever one happened to be alive.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

const CONVERSATION_ID = "conv-77";

const { pageStore } = vi.hoisted(() => {
	const value = {
		url: new URL("http://localhost/augur/conv-77"),
		params: { conversationId: "conv-77" },
	};
	return {
		pageStore: {
			subscribe(run: (next: typeof value) => void) {
				run(value);
				return () => {};
			},
		},
	};
});

vi.mock("$app/stores", () => ({ page: pageStore }));
vi.mock("$app/navigation", () => ({ replaceState: vi.fn() }));

type StreamCall = {
	options: { conversationId?: string };
	onComplete?: (result: {
		answer: string;
		citations: never[];
		relatedCitations: never[];
	}) => void;
};

const { getAugurConversation, streamCalls, streamAugurChat } = vi.hoisted(
	() => {
		const streamCalls: unknown[] = [];
		return {
			getAugurConversation: vi.fn(),
			streamCalls,
			streamAugurChat: vi.fn(
				(
					_transport: unknown,
					options: unknown,
					_onDelta?: unknown,
					_onThinking?: unknown,
					_onMeta?: unknown,
					onComplete?: unknown,
				) => {
					streamCalls.push({ options, onComplete });
					return new AbortController();
				},
			),
		};
	},
);

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	getAugurConversation,
	streamAugurChat,
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const calls = () => streamCalls as StreamCall[];

async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe("Augur conversation page", () => {
	beforeEach(() => {
		streamCalls.length = 0;
		getAugurConversation.mockResolvedValue({
			id: CONVERSATION_ID,
			title: "Tell me about quantum chips",
			createdAt: null,
			messages: [
				{
					role: "user",
					content: "Tell me about quantum chips",
					createdAt: null,
					citations: [],
					relatedCitations: [],
				},
				{
					role: "assistant",
					content: "Quantum chips use qubits.",
					createdAt: null,
					citations: [],
					relatedCitations: [],
				},
			],
		});
	});

	afterEach(() => {
		vi.clearAllMocks();
	});

	it("keeps a turn taken since the load through a rotation", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);

		const input = page.getByPlaceholder("What would you like to know?");
		await expect.element(input).toBeInTheDocument();

		// The turns the server sent survived a rotation even before, because
		// both chats were handed the same stored messages. What was lost was
		// everything said since — which is the part nothing can restore.
		await input.fill("And the error rates?");
		await page.getByRole("button", { name: /submit/i }).click();
		calls()[0]?.onComplete?.({
			answer: "About one in a thousand.",
			citations: [],
			relatedCitations: [],
		});
		await expect
			.element(page.getByText("About one in a thousand."))
			.toBeInTheDocument();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();
		await expect
			.element(page.getByText("And the error rates?"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("About one in a thousand."))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
		await expect
			.element(page.getByText("About one in a thousand."))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Quantum chips use qubits."))
			.toBeInTheDocument();

		// One fetch, one chat, one stream: rotating is not arriving.
		expect(getAugurConversation).toHaveBeenCalledTimes(1);
		expect(calls()).toHaveLength(1);
		expect(document.querySelectorAll("#augur-input")).toHaveLength(1);
	});

	it("threads a follow-up onto the conversation that was loaded", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);

		const input = page.getByPlaceholder("What would you like to know?");
		await expect.element(input).toBeInTheDocument();

		await input.fill("And the error rates?");
		await page.getByRole("button", { name: /submit/i }).click();

		expect(calls()).toHaveLength(1);
		expect(calls()[0]?.options.conversationId).toBe(CONVERSATION_ID);
	});
});
