import { tick } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import MorningLetterChat from "./MorningLetterChat.svelte";

describe("Desktop MorningLetterChat scroll timer cleanup", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("clears the 100ms scroll timer on unmount so advancing timers past 100ms does not throw or scroll after unmount", async () => {
		const { container, unmount } = render(MorningLetterChat);
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

		await tick();
		unmount();

		expect(() => {
			vi.advanceTimersByTime(150);
		}).not.toThrow();
		expect(scrollTopSetter).not.toHaveBeenCalled();
	});
});
