import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

const { pageState } = vi.hoisted(() => {
	const value: { url: URL; data: { userRole?: string } } = {
		url: new URL("http://localhost/home"),
		data: { userRole: "user" },
	};
	return {
		pageState: {
			get url() {
				return value.url;
			},
			set url(next: URL) {
				value.url = next;
			},
			get data() {
				return value.data;
			},
			set data(next: { userRole?: string }) {
				value.data = next;
			},
		},
	};
});

vi.mock("$app/state", () => ({ page: pageState }));

import Sidebar from "./Sidebar.svelte";

describe("Desktop Sidebar", () => {
	beforeEach(() => {
		pageState.url = new URL("http://localhost/home");
		pageState.data = { userRole: "user" };
	});

	it("renders primary navigation links including /search", async () => {
		render(Sidebar);

		const homeLink = page.getByRole("link", { name: /Knowledge Home/i });
		await expect.element(homeLink).toBeInTheDocument();
		await expect.element(homeLink).toHaveAttribute("href", "/home");

		const trailLink = page.getByRole("link", { name: "Your Trail" });
		await expect.element(trailLink).toBeInTheDocument();
		await expect.element(trailLink).toHaveAttribute("href", "/knowledge/trail");

		const searchLink = page.getByRole("link", { name: "Search" });
		await expect.element(searchLink).toBeInTheDocument();
		await expect.element(searchLink).toHaveAttribute("href", "/search");
	});

	it("surfaces /settings/notifications inside the Settings section", async () => {
		render(Sidebar);

		const settingsToggle = page.getByRole("button", { name: "Settings" });
		await expect.element(settingsToggle).toBeInTheDocument();
		await settingsToggle.click();

		const feedsLink = page.getByRole("link", { name: "Manage Feed Links" });
		await expect.element(feedsLink).toBeInTheDocument();
		await expect.element(feedsLink).toHaveAttribute("href", "/settings/feeds");

		const notificationsLink = page.getByRole("link", { name: "Notifications" });
		await expect.element(notificationsLink).toBeInTheDocument();
		await expect
			.element(notificationsLink)
			.toHaveAttribute("href", "/settings/notifications");
	});

	it("hides admin-gated links when isAdmin is false", async () => {
		render(Sidebar, { props: { isAdmin: false } });

		await expect
			.element(page.getByRole("button", { name: "Admin" }))
			.not.toBeInTheDocument();
	});

	it("shows admin-gated links when isAdmin is true", async () => {
		render(Sidebar, { props: { isAdmin: true } });

		const adminToggle = page.getByRole("button", { name: "Admin" });
		await expect.element(adminToggle).toBeInTheDocument();
		await adminToggle.click();

		const opsLink = page.getByRole("link", { name: "Knowledge Home Ops" });
		await expect.element(opsLink).toBeInTheDocument();
		await expect
			.element(opsLink)
			.toHaveAttribute("href", "/admin/knowledge-home");

		const monitorLink = page.getByRole("link", { name: "System Monitor" });
		await expect.element(monitorLink).toBeInTheDocument();
		await expect.element(monitorLink).toHaveAttribute("href", "/admin/monitor");
	});
});
