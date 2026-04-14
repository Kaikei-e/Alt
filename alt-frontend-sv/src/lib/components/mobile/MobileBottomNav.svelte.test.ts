import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import MobileBottomNav from "./MobileBottomNav.svelte";

describe("MobileBottomNav", () => {
	it("renders 5 tab labels", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/home" } });

		await expect.element(page.getByText("Home")).toBeInTheDocument();
		await expect.element(page.getByText("Swipe")).toBeInTheDocument();
		await expect.element(page.getByText("Search")).toBeInTheDocument();
		await expect.element(page.getByText("Augur")).toBeInTheDocument();
		await expect.element(page.getByText("Menu")).toBeInTheDocument();
	});

	it("renders 5 tab links with correct hrefs in order", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/home" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).toHaveAttribute("href", "/home");
		await expect
			.element(links.nth(1))
			.toHaveAttribute("href", "/feeds/swipe/visual-preview");
		await expect.element(links.nth(2)).toHaveAttribute("href", "/search");
		await expect.element(links.nth(3)).toHaveAttribute("href", "/augur");
		await expect.element(links.nth(4)).toHaveAttribute("href", "/menu");
	});

	it("marks the active tab with aria-current=page", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/augur" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(3)).toHaveAttribute("aria-current", "page");
	});

	it("does not set aria-current on inactive tabs", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/augur" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).not.toHaveAttribute("aria-current");
		await expect.element(links.nth(4)).not.toHaveAttribute("aria-current");
	});

	it("activates Feeds tab for /feeds/swipe", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/feeds/swipe" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(1)).toHaveAttribute("aria-current", "page");
	});

	it("activates Search tab for /feeds/search", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/feeds/search" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(2)).toHaveAttribute("aria-current", "page");
	});

	it("activates Menu tab for /recap (secondary destination)", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/recap" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(4)).toHaveAttribute("aria-current", "page");
	});

	it("is always rendered (no HIDE_PATHS)", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/augur" } });
		await expect.element(page.getByRole("navigation")).toBeInTheDocument();
	});

	it("renders on /feeds/search (bottom nav is persistent)", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/feeds/search" } });
		await expect.element(page.getByRole("navigation")).toBeInTheDocument();
	});

	it("has aria-label Main navigation on the nav element", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/home" } });
		await expect
			.element(page.getByRole("navigation"))
			.toHaveAttribute("aria-label", "Main navigation");
	});

	it("does not use role=tab on links (ARIA nav pattern, not tablist)", async () => {
		render(MobileBottomNav as never, { props: { pathname: "/home" } });
		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).not.toHaveAttribute("role");
	});
});
