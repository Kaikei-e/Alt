import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

// The page reads the query string off `$page`, and there is no router in
// browser mode to supply one. A hand-rolled writable store (the mock factory is
// hoisted above every import, `svelte/store` included) lets each test arrive at
// the page with the URL that test is about.
type PageValue = { url: URL };

const { pageStore } = vi.hoisted(() => {
	let value: { url: URL } = { url: new URL("http://localhost/augur") };
	const subscribers = new Set<(next: { url: URL }) => void>();
	return {
		pageStore: {
			subscribe(run: (next: { url: URL }) => void) {
				subscribers.add(run);
				run(value);
				return () => {
					subscribers.delete(run);
				};
			},
			set(next: { url: URL }) {
				value = next;
				for (const run of subscribers) run(value);
			},
		},
	};
});

vi.mock("$app/stores", () => ({ page: pageStore }));

// Shallow routing needs a live router; in browser mode there is none.
const { replaceState } = vi.hoisted(() => ({ replaceState: vi.fn() }));
vi.mock("$app/navigation", () => ({ replaceState }));

/** One recorded call to the model, with the handles the stream was given. */
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
				_onError?: unknown,
				_onProgress?: unknown,
				onConversationId?: unknown,
			) => {
				const controller = new AbortController();
				streamCalls.push({ options, controller, onComplete, onConversationId });
				return controller;
			},
		),
	};
});

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamAugurChat,
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const QUESTION = "What changed in AI chips today?";

const calls = () => streamCalls as StreamCall[];

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

function arriveWith(search = "") {
	const next: PageValue = { url: new URL(`http://localhost/augur${search}`) };
	pageStore.set(next);
}

const hasAugurClass = () =>
	document.documentElement.classList.contains("augur-page");

describe("Augur page overflow lock", () => {
	beforeEach(() => {
		arriveWith();
	});

	afterEach(() => {
		document.documentElement.classList.remove("augur-page");
	});

	it("locks <html> scrolling on a phone", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();

		expect(hasAugurClass()).toBe(true);
	});

	it("leaves <html> alone at a desktop width", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		expect(hasAugurClass()).toBe(false);
	});

	it("releases the lock when the phone is rotated into landscape", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();
		expect(hasAugurClass()).toBe(true);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		// `onMount` added the class once and only removed it on unmount, so the
		// desktop layout was left with `overflow: hidden` pinned on <html> and
		// nothing on the page able to scroll.
		expect(hasAugurClass()).toBe(false);
	});

	it("takes the lock when a desktop session is rotated onto a phone", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();
		expect(hasAugurClass()).toBe(false);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// The mirror of the case above: the phone shell is `position: fixed`, so
		// without the lock the document scrolls behind it.
		expect(hasAugurClass()).toBe(true);
	});

	it("releases the lock when the page goes away", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const { unmount } = render(Page);
		await settle();
		expect(hasAugurClass()).toBe(true);

		await unmount();
		expect(hasAugurClass()).toBe(false);
	});
});

// A question costs money and takes the reader's time. Turning the phone is not
// asking again — but the page used to hold two separately written chats and
// swap between them on the viewport, so every rotation destroyed the thread,
// mounted the other implementation, and let its own "have I sent this yet"
// guard start again from empty. One question, three answers billed, and the
// conversation gone each time.
describe("Augur page, question arriving in the URL", () => {
	beforeEach(() => {
		streamCalls.length = 0;
		streamAugurChat.mockClear();
		arriveWith(`?q=${encodeURIComponent(QUESTION)}`);
	});

	afterEach(() => {
		document.documentElement.classList.remove("augur-page");
	});

	it("asks the model once, however the phone is turned", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();
		expect(calls()).toHaveLength(1);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		expect(calls()).toHaveLength(1);
		expect(calls()[0]?.options.messages.at(-1)?.content).toBe(QUESTION);
	});

	it("keeps streaming the answer it started when the phone is turned", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// Aborting here throws away an answer the reader is already paying for.
		expect(calls()[0]?.controller.signal.aborted).toBe(false);
	});

	it("keeps the answered thread through a rotation", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		calls()[0]?.onComplete?.({
			answer: "Inference chips got cheaper.",
			citations: [],
			relatedCitations: [],
		});
		await expect
			.element(page.getByText("Inference chips got cheaper."))
			.toBeInTheDocument();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();
		await expect
			.element(page.getByText("Inference chips got cheaper."))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
		await expect
			.element(page.getByText("Inference chips got cheaper."))
			.toBeInTheDocument();
		await expect.element(page.getByText(QUESTION)).toBeInTheDocument();
	});

	it("abandons the stream when the page itself goes away", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		const { unmount } = render(Page);
		await settle();
		expect(calls()).toHaveLength(1);

		await unmount();

		expect(calls()[0]?.controller.signal.aborted).toBe(true);
	});

	it("puts the persisted conversation id in the URL through the router", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();

		calls()[0]?.onConversationId?.("conv-42");

		// Raw `history.replaceState` is the one call SvelteKit's router asks
		// callers not to make; and on a phone the id used to be dropped on the
		// floor entirely, so a reload lost the conversation.
		expect(replaceState).toHaveBeenCalledWith("/augur/conv-42", {});
	});
});
