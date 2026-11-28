import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { NAV_TABS } from "./bottom-nav";
import MobileBottomNav from "./MobileBottomNav.svelte";

describe("MobileBottomNav", () => {
	it("renders one labelled tab per NAV_TABS entry", async () => {
		render(MobileBottomNav, { props: { pathname: "/today" } });

		const nav = page.getByRole("navigation");
		await expect.element(nav).toBeInTheDocument();
		expect(nav.getByRole("link").elements()).toHaveLength(NAV_TABS.length);

		for (const tab of NAV_TABS) {
			await expect.element(page.getByText(tab.label)).toBeInTheDocument();
		}
	});

	it("renders the tab links in NAV_TABS order with their declared hrefs", async () => {
		render(MobileBottomNav, { props: { pathname: "/today" } });

		const links = page.getByRole("navigation").getByRole("link");
		for (const [i, tab] of NAV_TABS.entries()) {
			await expect.element(links.nth(i)).toHaveAttribute("href", tab.href);
		}
	});

	it("marks the Today tab with aria-current=page on /today", async () => {
		render(MobileBottomNav, { props: { pathname: "/today" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).toHaveAttribute("aria-current", "page");
		await expect.element(links.nth(4)).not.toHaveAttribute("aria-current");
	});

	it("activates Gallery tab for /feeds/visual-preview", async () => {
		render(MobileBottomNav, { props: { pathname: "/feeds/visual-preview" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(2)).toHaveAttribute("aria-current", "page");
	});

	it("activates Themes tab for /knowledge", async () => {
		render(MobileBottomNav, { props: { pathname: "/knowledge" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(3)).toHaveAttribute("aria-current", "page");
	});

	it("activates Menu tab for secondary destinations (/search, /recap, /augur)", async () => {
		render(MobileBottomNav, { props: { pathname: "/recap" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(4)).toHaveAttribute("aria-current", "page");
	});

	it("renders unconditionally (hiding is ResponsiveLayout's job)", async () => {
		render(MobileBottomNav, { props: { pathname: "/today" } });
		await expect.element(page.getByRole("navigation")).toBeInTheDocument();
	});

	it("renders on /feeds/search (bottom nav is persistent)", async () => {
		render(MobileBottomNav, { props: { pathname: "/feeds/search" } });
		await expect.element(page.getByRole("navigation")).toBeInTheDocument();
	});

	it("has aria-label Main navigation on the nav element", async () => {
		render(MobileBottomNav, { props: { pathname: "/today" } });
		await expect
			.element(page.getByRole("navigation"))
			.toHaveAttribute("aria-label", "Main navigation");
	});

	it("does not use role=tab on links (ARIA nav pattern, not tablist)", async () => {
		render(MobileBottomNav, { props: { pathname: "/today" } });
		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).not.toHaveAttribute("role");
	});
});
