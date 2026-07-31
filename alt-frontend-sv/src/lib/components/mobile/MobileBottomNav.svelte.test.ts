import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { NAV_TABS } from "./bottom-nav";
import MobileBottomNav from "./MobileBottomNav.svelte";

// The tab list is derived from NAV_TABS rather than duplicated here: ADR-000716
// makes `src/lib/config/navigation.ts` the single source of truth for the nav,
// and this file's job is that the component renders exactly what that source
// declares. What each tab *is* stays pinned in bottom-nav.test.ts /
// navigation.test.ts, so a nav change fails in one place instead of silently
// drifting away from a copy kept here — which is what happened to the "Swipe"
// tab this spec used to assert: ADR-000903 replaced it at index 1 (Loop, later
// Trail per ADR-000940), keeping 5 tabs and moving the swipe destination to the
// Menu page's Browse section, and ADR-000948 then made /feeds/swipe* immersive
// so ResponsiveLayout drops the bar there entirely.

describe("MobileBottomNav", () => {
	it("renders one labelled tab per NAV_TABS entry", async () => {
		render(MobileBottomNav, { props: { pathname: "/home" } });

		const nav = page.getByRole("navigation");
		await expect.element(nav).toBeInTheDocument();
		expect(nav.getByRole("link").elements()).toHaveLength(NAV_TABS.length);

		for (const tab of NAV_TABS) {
			await expect.element(page.getByText(tab.label)).toBeInTheDocument();
		}
	});

	it("renders the tab links in NAV_TABS order with their declared hrefs", async () => {
		render(MobileBottomNav, { props: { pathname: "/home" } });

		const links = page.getByRole("navigation").getByRole("link");
		for (const [i, tab] of NAV_TABS.entries()) {
			await expect.element(links.nth(i)).toHaveAttribute("href", tab.href);
		}
	});

	it("marks the active tab with aria-current=page", async () => {
		render(MobileBottomNav, { props: { pathname: "/augur" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(3)).toHaveAttribute("aria-current", "page");
	});

	it("does not set aria-current on inactive tabs", async () => {
		render(MobileBottomNav, { props: { pathname: "/augur" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).not.toHaveAttribute("aria-current");
		await expect.element(links.nth(4)).not.toHaveAttribute("aria-current");
	});

	it("activates Trail tab for /knowledge/trail", async () => {
		render(MobileBottomNav, { props: { pathname: "/knowledge/trail" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(1)).toHaveAttribute("aria-current", "page");
	});

	it("activates Search tab for /feeds/search", async () => {
		render(MobileBottomNav, { props: { pathname: "/feeds/search" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(2)).toHaveAttribute("aria-current", "page");
	});

	it("activates Menu tab for /recap (secondary destination)", async () => {
		render(MobileBottomNav, { props: { pathname: "/recap" } });

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(4)).toHaveAttribute("aria-current", "page");
	});

	// The component itself has no hide list: immersive routes (/feeds/swipe*)
	// are dropped by ResponsiveLayout, not by this component (ADR-000948).
	it("renders unconditionally (hiding is ResponsiveLayout's job)", async () => {
		render(MobileBottomNav, { props: { pathname: "/augur" } });
		await expect.element(page.getByRole("navigation")).toBeInTheDocument();
	});

	it("renders on /feeds/search (bottom nav is persistent)", async () => {
		render(MobileBottomNav, { props: { pathname: "/feeds/search" } });
		await expect.element(page.getByRole("navigation")).toBeInTheDocument();
	});

	it("has aria-label Main navigation on the nav element", async () => {
		render(MobileBottomNav, { props: { pathname: "/home" } });
		await expect
			.element(page.getByRole("navigation"))
			.toHaveAttribute("aria-label", "Main navigation");
	});

	it("does not use role=tab on links (ARIA nav pattern, not tablist)", async () => {
		render(MobileBottomNav, { props: { pathname: "/home" } });
		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).not.toHaveAttribute("role");
	});
});
