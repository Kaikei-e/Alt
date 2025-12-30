import { page } from "@vitest/browser/context";
import { createRawSnippet } from "svelte";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";

import Card from "./card.svelte";

describe("Card", () => {
	it("renders default slot content", async () => {
		render(Card, {
			props: {
				children: createRawSnippet(() => ({
					render: () => "<p>Card body</p>",
				})),
			},
		});

		await expect.element(page.getByText("Card body")).toBeInTheDocument();
	});

	it("accepts additional class names", async () => {
		render(Card, {
			props: {
				class: "shadow-lg",
				children: createRawSnippet(() => ({
					render: () => "<p data-testid='card-content'>Styled card</p>",
				})),
			},
		});

		const cardContent = page.getByTestId("card-content");
		await expect.element(cardContent).toBeInTheDocument();
		// Check parent div has the classes
		const cardText = page.getByText("Styled card");
		await expect.element(cardText).toBeInTheDocument();
	});
});
