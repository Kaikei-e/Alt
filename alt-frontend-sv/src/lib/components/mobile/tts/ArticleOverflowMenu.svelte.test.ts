import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import ArticleOverflowMenu from "./ArticleOverflowMenu.svelte";

describe("ArticleOverflowMenu", () => {
	it("renders a closed trigger by default", async () => {
		render(ArticleOverflowMenu, {
			props: { onListenSelect: vi.fn() },
		});

		const trigger = page.getByRole("button", { name: /article actions/i });
		await expect.element(trigger).toBeInTheDocument();
		await expect.element(trigger).toHaveAttribute("aria-haspopup", "menu");
		await expect.element(trigger).toHaveAttribute("aria-expanded", "false");
	});

	it("opens the menu and shows the Listen item on click", async () => {
		render(ArticleOverflowMenu, {
			props: { onListenSelect: vi.fn() },
		});

		const trigger = page.getByRole("button", { name: /article actions/i });
		await trigger.click();

		await expect.element(trigger).toHaveAttribute("aria-expanded", "true");
		await expect
			.element(page.getByRole("menuitem", { name: /listen to article/i }))
			.toBeInTheDocument();
	});

	it("invokes onListenSelect when the Listen item is activated", async () => {
		const onListenSelect = vi.fn();
		render(ArticleOverflowMenu, { props: { onListenSelect } });

		await page.getByRole("button", { name: /article actions/i }).click();
		await page.getByRole("menuitem", { name: /listen to article/i }).click();

		expect(onListenSelect).toHaveBeenCalledOnce();
	});
});
