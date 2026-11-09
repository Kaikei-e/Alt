import { tick } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamMorningLetterChat: vi.fn(() => new AbortController()),
}));

import MorningLetterChat from "./MorningLetterChat.svelte";

describe("Mobile MorningLetterChat scroll timer cleanup", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("clears the 100ms scroll timer on unmount so advancing timers past 100ms does not throw or scroll after unmount", async () => {
		const { container, unmount } = render(MorningLetterChat);
		const toggleButton =
			container.querySelector<HTMLButtonElement>(".disclosure-toggle");
		expect(toggleButton).not.toBeNull();
		toggleButton?.click();
		await tick();

		const threadEl = container.querySelector(".letter-thread");
		expect(threadEl).not.toBeNull();

		const scrollTopSetter = vi.fn();
		if (threadEl) {
			Object.defineProperty(threadEl, "scrollTop", {
				set: scrollTopSetter,
				get: () => 0,
				configurable: true,
			});
		}

		const inputEl = container.querySelector<HTMLInputElement>(".input-field");
		expect(inputEl).not.toBeNull();
		if (inputEl) {
			inputEl.value = "What happened?";
			inputEl.dispatchEvent(new Event("input", { bubbles: true }));
		}
		await tick();

		const sendButton =
			container.querySelector<HTMLButtonElement>(".input-submit");
		expect(sendButton).not.toBeNull();
		sendButton?.click();
		await tick();

		unmount();

		expect(() => {
			vi.advanceTimersByTime(150);
		}).not.toThrow();
		expect(scrollTopSetter).not.toHaveBeenCalled();
	});
});
