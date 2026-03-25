import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ChatWindow from "./ChatWindow.svelte";

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamAugurChat: vi.fn(() => new AbortController()),
}));

describe("ChatWindow", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		// Mock fetch for browser environment
		vi.stubGlobal("fetch", vi.fn());
	});

	it("renders correctly", async () => {
		render(ChatWindow);
		await expect
			.element(page.getByPlaceholder("Type your message..."))
			.toBeInTheDocument();
	});

	it("sends a message and displays user message", async () => {
		render(ChatWindow);
		const input = page.getByPlaceholder("Type your message...");
		const button = page.getByRole("button", { name: /send/i });

		await input.fill("Hello Augur");
		await button.click();

		// Input should be cleared
		await expect.element(input).toHaveValue("");
		// User message should be displayed
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
});
