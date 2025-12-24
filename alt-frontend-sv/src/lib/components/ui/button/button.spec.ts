import { render, screen } from "@testing-library/svelte/svelte5";
import userEvent from "@testing-library/user-event";
import { createRawSnippet } from "svelte";
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

		const actionButton = screen.getByRole("button", { name: /confirm/i });
		expect(actionButton).toBeInTheDocument();
		expect(actionButton).toBeEnabled();
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

		const link = screen.getByRole("link", { name: /visit/i });
		expect(link).toHaveAttribute("href", "https://alt.ai");
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

		const disabledLink = screen.getByRole("link", { name: /visit/i });
		expect(disabledLink).not.toHaveAttribute("href");
		expect(disabledLink).toHaveAttribute("aria-disabled", "true");
		expect(disabledLink).toHaveAttribute("tabindex", "-1");
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

		const actionButton = screen.getByRole("button", { name: /send/i });
		await userEvent.click(actionButton);

		expect(clickHandler).toHaveBeenCalled();
	});
});
