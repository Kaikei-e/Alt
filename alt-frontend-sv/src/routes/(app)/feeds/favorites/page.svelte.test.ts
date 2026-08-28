import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { getFavoriteFeedsWithCursorClient } from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";

// Spy-mode automocks keep every export bound and let each test override only
// the network calls.
vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$lib/api/client", { spy: true });

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const clipping: RenderFeed = {
	id: "feed-1",
	title: "A Clipped Dispatch",
	description: "kept for later",
	link: "https://example.com/clipped",
	published: "2026-08-01T00:00:00Z",
	publishedAtFormatted: "Aug 1, 2026",
	mergedTagsLabel: "",
	normalizedUrl: "https://example.com/clipped",
	excerpt: "kept for later",
};

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 80));
}

describe("Clippings File page", () => {
	beforeEach(() => {
		vi.mocked(getFavoriteFeedsWithCursorClient).mockReset();
		vi.mocked(getFavoriteFeedsWithCursorClient).mockResolvedValue({
			data: [clipping],
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<
			ReturnType<typeof getFavoriteFeedsWithCursorClient>
		>);
	});

	it("lists the clippings when the app opens on a phone", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);

		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();
	});

	it("lists the clippings after a landscape start is rotated upright", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// The phone list was fetched in `onMount` behind an `isMobile()` guard,
		// so a session that started in landscape never fetched it — and
		// `mobileIsLoading` starts as `true`, leaving "Retrieving your
		// clippings…" on screen with nothing on its way.
		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();
	});

	it("keeps the phone list across a round trip instead of dropping back to loading", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// Rotating away and back must not reset the list to its loading state:
		// the fetch is a once-per-side job, not a once-per-rotation one.
		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(/Retrieving your clippings/))
			.not.toBeInTheDocument();
	});
});
