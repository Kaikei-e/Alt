import { page } from "@vitest/browser/context";
import { createRawSnippet } from "svelte";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";

import Button from "./button.svelte";

describe("Button", () => {
	it("renders a button element with accessible name and enabled state", async () => {
		render(Button, {
			props: {
				children: createRawSnippet(() => ({
					render: () => "<span>Confirm</span>",
				})),
			},
		});

		const actionButton = page.getByRole("button", { name: /confirm/i });
		await expect.element(actionButton).toBeInTheDocument();
		await expect.element(actionButton).toBeEnabled();
	});

	it("renders an anchor when href is provided", async () => {
		render(Button, {
			props: {
				href: "https://alt.ai",
				children: createRawSnippet(() => ({
					render: () => "<span>Visit</span>",
				})),
			},
		});

		const link = page.getByRole("link", { name: /visit/i });
		await expect.element(link).toHaveAttribute("href", "https://alt.ai");
	});

	it("prevents navigation when disabled on an anchor", async () => {
		render(Button, {
			props: {
				href: "https://alt.ai",
				disabled: true,
				children: createRawSnippet(() => ({
					render: () => "<span>Visit</span>",
				})),
			},
		});

		const disabledLink = page.getByRole("link", { name: /visit/i });
		await expect.element(disabledLink).not.toHaveAttribute("href");
		await expect.element(disabledLink).toHaveAttribute("aria-disabled", "true");
		await expect.element(disabledLink).toHaveAttribute("tabindex", "-1");
	});

	it("emits a click event when clicked", async () => {
		const clickHandler = vi.fn();
		render(Button, {
			props: {
				onclick: clickHandler,
				children: createRawSnippet(() => ({
					render: () => "<span>Send</span>",
				})),
			},
		});

		const actionButton = page.getByRole("button", { name: /send/i });
		await actionButton.click();

		expect(clickHandler).toHaveBeenCalled();
	});
});
