import { render, screen } from "@testing-library/svelte/svelte5";
import { createRawSnippet } from "svelte";
import { describe, expect, it } from "vitest";

import Card from "./card.svelte";

describe("Card", () => {
	it("renders default slot content", () => {
		render(Card, {
			props: {
				children: createRawSnippet(() => ({
					render: () => "<p>Card body</p>",
				})),
			},
		});

		expect(screen.getByText("Card body")).toBeInTheDocument();
	});

	it("accepts additional class names", () => {
		render(Card, {
			props: {
				class: "shadow-lg",
				children: createRawSnippet(() => ({
					render: () => "<p>Styled card</p>",
				})),
			},
		});

		const cardElement = screen.getByText("Styled card").closest("div");
		expect(cardElement).toHaveClass("border-2");
		expect(cardElement).toHaveClass("shadow-lg");
	});
});
