import { readable } from "svelte/store";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

// The page only reads the query string off `$page`, and there is no router in
// browser mode to supply one.
vi.mock("$app/stores", () => ({
	page: readable({ url: new URL("http://localhost/augur") }),
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

/** The media query listener fires off a browser event, so yield a frame. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

const hasAugurClass = () =>
	document.documentElement.classList.contains("augur-page");

describe("Augur page overflow lock", () => {
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
