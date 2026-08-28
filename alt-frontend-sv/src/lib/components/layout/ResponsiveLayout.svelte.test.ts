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

// A rotation must swap the *chrome* (sidebar vs. bottom nav) without rebuilding
// the page underneath it. When each viewport branch owned its own `<main>` with
// its own `{@render children()}`, flipping the branch destroyed and re-created
// the whole route component tree — every page, including the ones that never
// read the viewport. The damage that caused is not visible in the markup, so
// these assert on node identity: mark the live nodes, rotate, and require the
// marks to still be there.
describe("ResponsiveLayout page continuity across a rotation", () => {
	function probe(el: Element | null, mark: string): HTMLElement {
		expect(el).not.toBeNull();
		(el as HTMLElement).dataset.identityProbe = mark;
		return el as HTMLElement;
	}

	it("keeps the same <main> element when the phone is rotated into landscape", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const { container } = render(ResponsiveLayout, {
			props: { children: body },
		});
		const before = probe(container.querySelector("main"), "portrait");

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();

		const after = container.querySelector("main");
		expect(after).toBe(before);
		expect((after as HTMLElement).dataset.identityProbe).toBe("portrait");
	});

	it("keeps the same <main> element when the phone is rotated upright again", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		const { container } = render(ResponsiveLayout, {
			props: { children: body },
		});
		const before = probe(container.querySelector("main"), "landscape");

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		const after = container.querySelector("main");
		expect(after).toBe(before);
		expect((after as HTMLElement).dataset.identityProbe).toBe("landscape");
	});

	it("keeps the rendered page body mounted across a round trip", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const { container } = render(ResponsiveLayout, {
			props: { children: body },
		});
		const before = probe(
			container.querySelector('[data-testid="page-body"]'),
			"same-tree",
		);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		const after = container.querySelector('[data-testid="page-body"]');
		expect(after).toBe(before);
		expect((after as HTMLElement).dataset.identityProbe).toBe("same-tree");
	});

	it("exposes exactly one skip-link target at either width", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const { container } = render(ResponsiveLayout, {
			props: { children: body },
		});

		const assertSingleMain = () => {
			const mains = container.querySelectorAll("main");
			expect(mains).toHaveLength(1);
			const main = mains[0] as HTMLElement;
			// The skip link hrefs "#main" and focus is moved to this node after
			// every navigation, so both attributes have to survive the merge.
			expect(main.id).toBe("main");
			expect(main.getAttribute("tabindex")).toBe("-1");
			expect(
				container.querySelector<HTMLAnchorElement>("a.skip-link")?.hash,
			).toBe("#main");
		};

		assertSingleMain();
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
		assertSingleMain();
	});
});
