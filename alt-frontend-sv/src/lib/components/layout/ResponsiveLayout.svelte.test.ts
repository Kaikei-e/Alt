import { createRawSnippet } from "svelte";
import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import ResponsiveLayout from "./ResponsiveLayout.svelte";

// 393 x 851 is a Pixel 5 held upright; rotating it makes the viewport 851 wide,
// which is past the 768px desktop breakpoint. A reader who opens the app in
// landscape, or turns the phone while reading, must get the layout that matches
// the width they actually have — not the one that happened to be true when the
// component first ran.
const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const body = createRawSnippet(() => ({
	render: () => `<p data-testid="page-body">body</p>`,
}));

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe("ResponsiveLayout", () => {
	it("shows the mobile bottom nav on a portrait phone", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(ResponsiveLayout, { props: { children: body } });

		await expect
			.element(page.getByRole("navigation", { name: "Main navigation" }))
			.toBeInTheDocument();
	});

	it("shows the desktop sidebar when the app opens at a desktop width", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(ResponsiveLayout, { props: { children: body } });

		await expect.element(page.getByText("Alt Reader")).toBeInTheDocument();
	});

	it("swaps to the desktop layout when the phone is rotated into landscape", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(ResponsiveLayout, { props: { children: body } });
		await expect
			.element(page.getByRole("navigation", { name: "Main navigation" }))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		await expect.element(page.getByText("Alt Reader")).toBeInTheDocument();
	});

	it("swaps back to the mobile layout when the phone is rotated upright again", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(ResponsiveLayout, { props: { children: body } });
		await expect.element(page.getByText("Alt Reader")).toBeInTheDocument();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		await expect
			.element(page.getByRole("navigation", { name: "Main navigation" }))
			.toBeInTheDocument();
	});
});
