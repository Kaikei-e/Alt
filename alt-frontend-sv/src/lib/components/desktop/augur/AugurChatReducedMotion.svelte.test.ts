/**
 * AugurChat under prefers-reduced-motion.
 *
 * Lives apart from AugurChat.svelte.test.ts because a vi.mock of the motion
 * store is file-scoped: the main suite needs the real (motion-allowed) answer,
 * this one needs the reduced answer.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

type StreamCall = {
	onDelta?: (text: string) => void;
};

const { streamCalls, streamAugurChat } = vi.hoisted(() => {
	const streamCalls: unknown[] = [];
	return {
		streamCalls,
		streamAugurChat: vi.fn(
			(
				_transport: unknown,
				_options: unknown,
				onDelta?: unknown,
				..._rest: unknown[]
			) => {
				streamCalls.push({ onDelta });
				return new AbortController();
			},
		),
	};
});

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamAugurChat,
}));

vi.mock("$lib/stores/motion.svelte", () => ({
	prefersReducedMotion: () => true,
}));

import AugurChat from "./AugurChat.svelte";

const calls = () => streamCalls as StreamCall[];
const input = () => page.getByPlaceholder("What would you like to know?");
const submit = () => page.getByRole("button", { name: /submit/i });

describe("AugurChat with prefers-reduced-motion", () => {
	beforeEach(async () => {
		streamCalls.length = 0;
		streamAugurChat.mockClear();
		await page.viewport(1280, 900);
	});

	it("shows each delta whole, without ever scheduling an animation frame", async () => {
		render(AugurChat);

		await input().fill("No motion please");
		await submit().click();

		const full = "steady text ".repeat(20).trim();
		const rafSpy = vi.spyOn(window, "requestAnimationFrame");
		try {
			calls()[0]?.onDelta?.(full);

			// The push itself is synchronous in immediate mode: no frame was
			// requested to carry the reveal.
			expect(rafSpy).not.toHaveBeenCalled();

			await expect
				.element(page.getByText(full, { exact: true }))
				.toBeInTheDocument();
		} finally {
			rafSpy.mockRestore();
		}
	});
});
