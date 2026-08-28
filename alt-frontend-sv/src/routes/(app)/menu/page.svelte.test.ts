import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { goto } from "$app/navigation";

// Spy-mode automock rather than a factory: a factory replaces the whole module
// and any export it forgets becomes an import-time SyntaxError. Only `goto`
// must not really run — real navigation would steer the browser-mode page away
// mid-test.
vi.mock("$app/navigation", { spy: true });

import Page from "./+page.svelte";

// A Pixel 5 is 393x851 upright; rotated it is 851 wide, past the 768px
// breakpoint. Both orientations are the same phone, so a rotation must not
// leave the reader anywhere they cannot get out of.
const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe("Menu page", () => {
	beforeEach(() => {
		vi.mocked(goto).mockReset();
		vi.mocked(goto).mockImplementation(async () => {});
	});

	it("hands a desktop arrival over to /feeds, where the sidebar carries the same links", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		expect(goto).toHaveBeenCalledWith("/feeds", { replaceState: true });
	});

	it("keeps the menu readable when the phone is rotated into landscape", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await expect
			.element(page.getByRole("heading", { name: "Menu" }))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		// The desktop branch used to be an empty `<div>` whose only escape was
		// the `onMount` redirect — and `onMount` does not run again for a
		// rotation, so the reader was left staring at a white screen.
		await expect
			.element(page.getByRole("heading", { name: "Menu" }))
			.toBeInTheDocument();
	});

	it("does not navigate just because the phone was rotated", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();
		expect(goto).not.toHaveBeenCalled();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		// Turning a phone is not an arrival. A `goto` in an `$effect` would fire
		// here, and would also yank a desktop reader away every time a window
		// was dragged across 768px.
		expect(goto).not.toHaveBeenCalled();
	});
});
