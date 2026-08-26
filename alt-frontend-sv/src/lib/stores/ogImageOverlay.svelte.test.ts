import { beforeEach, describe, expect, it } from "vitest";
import { createOgImageOverlay } from "./ogImageOverlay.svelte";

describe("createOgImageOverlay", () => {
	let overlay: ReturnType<typeof createOgImageOverlay>;

	beforeEach(() => {
		overlay = createOgImageOverlay();
	});

	it("has nothing to say about an article nobody asked about", () => {
		expect(overlay.cell("article-1")?.url).toBeNull();
		expect(overlay.has("article-1")).toBe(false);
	});

	it("hands back a URL that arrived for an article", () => {
		overlay.resolve("article-1", "/proxy/one");

		expect(overlay.cell("article-1")?.url).toBe("/proxy/one");
		expect(overlay.has("article-1")).toBe(true);
	});

	it("gives the same cell back, so a card holds one reference for its lifetime", () => {
		// The card takes its cell once at init. A fresh cell per read would
		// mean the answer landed somewhere nothing is rendering.
		expect(overlay.cell("article-1")).toBe(overlay.cell("article-1"));
	});

	it("delivers into a cell taken before the answer arrived", () => {
		// This is the whole point: the card asked first, the URL came second.
		const cell = overlay.cell("article-1");
		expect(cell?.url).toBeNull();

		overlay.resolve("article-1", "/proxy/late");

		expect(cell?.url).toBe("/proxy/late");
	});

	it("tolerates a missing key rather than answering for the wrong article", () => {
		expect(overlay.cell(undefined)).toBeNull();
		expect(overlay.cell("")).toBeNull();
	});

	it("ignores an empty URL instead of storing a blank answer", () => {
		// "" is not an image. Storing it would make the cell truthy and pin the
		// card to a load that can only fail.
		overlay.resolve("article-1", "");

		expect(overlay.cell("article-1")?.url).toBeNull();
		expect(overlay.has("article-1")).toBe(false);
	});

	it("keeps two articles apart", () => {
		overlay.resolve("article-1", "/proxy/one");

		expect(overlay.cell("article-2")?.url).toBeNull();
		expect(overlay.has("article-2")).toBe(false);
	});
});
