import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import MobileBottomNav from "./MobileBottomNav.svelte";

describe("MobileBottomNav", () => {
	it("renders 5 tabs with correct labels", async () => {
		render(MobileBottomNav as never, {
			props: { pathname: "/home" },
		});

		await expect.element(page.getByText("Home")).toBeInTheDocument();
		await expect.element(page.getByText("Search")).toBeInTheDocument();
		await expect.element(page.getByText("Recap")).toBeInTheDocument();
		await expect.element(page.getByText("Explore")).toBeInTheDocument();
		await expect.element(page.getByText("Library")).toBeInTheDocument();
	});

	it("renders 5 tab links with correct hrefs", async () => {
		render(MobileBottomNav as never, {
			props: { pathname: "/home" },
		});

		const nav = page.getByRole("navigation");
		await expect.element(nav).toBeInTheDocument();

		const links = nav.getByRole("link");
		await expect.element(links.nth(0)).toHaveAttribute("href", "/home");
		await expect.element(links.nth(1)).toHaveAttribute("href", "/search");
		await expect.element(links.nth(2)).toHaveAttribute("href", "/recap");
		await expect
			.element(links.nth(3))
			.toHaveAttribute("href", "/feeds/tag-trail");
		await expect.element(links.nth(4)).toHaveAttribute("href", "/feeds");
	});

	it("highlights the active tab based on pathname", async () => {
		render(MobileBottomNav as never, {
			props: { pathname: "/recap" },
		});

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");

		// Recap tab (index 2) should have active styling
		await expect.element(links.nth(2)).toHaveAttribute("data-active", "true");
		// Home tab should not be active
		await expect.element(links.nth(0)).toHaveAttribute("data-active", "false");
	});

	it("activates Explore for /feeds/tag-trail paths", async () => {
		render(MobileBottomNav as never, {
			props: { pathname: "/feeds/tag-trail" },
		});

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");

		// Explore tab (index 3) should be active, not Library (index 4)
		await expect.element(links.nth(3)).toHaveAttribute("data-active", "true");
		await expect.element(links.nth(4)).toHaveAttribute("data-active", "false");
	});

	it("activates Library for /feeds paths", async () => {
		render(MobileBottomNav as never, {
			props: { pathname: "/feeds" },
		});

		const nav = page.getByRole("navigation");
		const links = nav.getByRole("link");

		// Library tab (index 4) should be active
		await expect.element(links.nth(4)).toHaveAttribute("data-active", "true");
		// Explore should not be active
		await expect.element(links.nth(3)).toHaveAttribute("data-active", "false");
	});

	it("renders nothing when pathname is a hidden route", async () => {
		const { container } = render(MobileBottomNav as never, {
			props: { pathname: "/augur" },
		});

		expect(container.innerHTML.trim()).toBe("");
	});

	it("renders nothing for /feeds/swipe", async () => {
		const { container } = render(MobileBottomNav as never, {
			props: { pathname: "/feeds/swipe" },
		});

		expect(container.innerHTML.trim()).toBe("");
	});

	it("renders nothing for /feeds/search", async () => {
		const { container } = render(MobileBottomNav as never, {
			props: { pathname: "/feeds/search" },
		});

		expect(container.innerHTML.trim()).toBe("");
	});

	it("has aria-label on the nav element", async () => {
		render(MobileBottomNav as never, {
			props: { pathname: "/home" },
		});

		await expect
			.element(page.getByRole("navigation"))
			.toHaveAttribute("aria-label", "Main navigation");
	});
});
