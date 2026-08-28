import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { goto } from "$app/navigation";

// Spy-mode automock: only `goto` must not really run.
vi.mock("$app/navigation", { spy: true });

// The desktop branch mounts a WebGL scene through Threlte, whose dev tooling
// pulls a build of tweakpane this bundler cannot resolve — the import error
// takes the whole file down before a test runs. Nothing here asserts on the
// scene; a no-op component is exactly what a Svelte 5 client component is.
vi.mock("$lib/components/desktop/tag-verse/TagVerseScreen.svelte", () => ({
	default: () => {},
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe("Tag Verse page", () => {
	beforeEach(() => {
		vi.mocked(goto).mockReset();
		vi.mocked(goto).mockImplementation(async () => {});
	});

	it("sends a phone-sized arrival to Knowledge Home", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();

		expect(goto).toHaveBeenCalledWith("/home", { replaceState: true });
	});

	it("gives the phone notice a way onward", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);

		// The notice is what a reader sees when the redirect is not what
		// happened — a rotation, or a redirect that did not land — so it has to
		// carry its own exit rather than assume one.
		await expect
			.element(page.getByRole("link", { name: /knowledge home/i }))
			.toBeInTheDocument();
	});

	it("does not navigate just because the window was narrowed", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();
		expect(goto).not.toHaveBeenCalled();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// Dragging a desktop window across 768px must not throw the reader out
		// of the page; the notice above is the answer instead.
		expect(goto).not.toHaveBeenCalled();
		await expect
			.element(page.getByRole("link", { name: /knowledge home/i }))
			.toBeInTheDocument();
	});
});
