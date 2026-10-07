import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { getFeedsWithCursorClient } from "#lib/api/client/feeds.js";
import { createClientTransport, getThreeDayRecap } from "#lib/connect/index.js";
import { useFeedStats } from "#lib/hooks/useFeedStats.svelte.js";
import { goto } from "$app/navigation";

// Spy-mode automocks keep every export bound (a factory that enumerates a
// subset turns anything it omits into an import-time SyntaxError) and let each
// test override only what must not really run: navigation and the network.
vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));
vi.mock("#lib/api/client/feeds.js", { spy: true });
vi.mock("#lib/connect/index.js", { spy: true });
vi.mock("#lib/hooks/useFeedStats.svelte.js", { spy: true });

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe("Dashboard page", () => {
	beforeEach(() => {
		vi.mocked(goto).mockReset();
		vi.mocked(goto).mockImplementation(async () => {});
		vi.mocked(getFeedsWithCursorClient).mockResolvedValue({
			data: [],
			next_cursor: undefined,
		} as unknown as Awaited<ReturnType<typeof getFeedsWithCursorClient>>);
		vi.mocked(createClientTransport).mockReturnValue(
			{} as ReturnType<typeof createClientTransport>,
		);
		vi.mocked(getThreeDayRecap).mockResolvedValue(
			null as unknown as Awaited<ReturnType<typeof getThreeDayRecap>>,
		);
		vi.mocked(useFeedStats).mockReturnValue({
			feedAmount: 0,
			unsummarizedArticlesAmount: 0,
			totalArticlesAmount: 0,
			isConnected: false,
			retryCount: 0,
			reconnect: () => {},
		});
	});

	it("sends a phone-sized arrival to Knowledge Home", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();

		expect(goto).toHaveBeenCalledWith("/home", { replace: true });
	});

	it("renders the brief when the app opens at a desktop width", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);

		await expect
			.element(page.getByRole("heading", { name: "Editorial Brief" }))
			.toBeInTheDocument();
		expect(goto).not.toHaveBeenCalled();
	});

	it("offers a way out instead of stalling when the phone is rotated upright", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await expect
			.element(page.getByRole("heading", { name: "Editorial Brief" }))
			.toBeInTheDocument();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// The narrow branch used to say "Redirecting…" and lean on the `onMount`
		// redirect to make that true. `onMount` does not run again for a
		// rotation, so the sentence became a permanent lie with nothing on
		// screen to act on. Whatever it says now, it has to be actionable.
		await expect
			.element(page.getByRole("link", { name: /knowledge home/i }))
			.toBeInTheDocument();
	});

	it("does not navigate just because the phone was rotated", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();
		expect(goto).not.toHaveBeenCalled();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// A resize is not an arrival: redirecting here would throw away a brief
		// the reader is part-way through every time a window crossed 768px.
		expect(goto).not.toHaveBeenCalled();
	});
});
