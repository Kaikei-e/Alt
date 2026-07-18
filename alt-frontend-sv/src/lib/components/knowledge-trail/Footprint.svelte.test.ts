import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { FootprintData } from "$lib/connect/knowledge_trail";
import Footprint from "./Footprint.svelte";

function makeFootprint(overrides: Partial<FootprintData> = {}): FootprintData {
	return {
		footprintKey: "open:article:abc",
		verb: "read",
		itemKey: "article:11111111-2222-3333-4444-555555555555",
		title: "Hunting Submarines",
		excerpt: "An article about gravity.",
		tags: ["submarine"],
		note: "",
		occurredAt: "2026-06-11T13:00:00Z",
		wear: "thin",
		contactCount: 1,
		firstOccurredAt: "2026-06-11T13:00:00Z",
		...overrides,
	};
}

describe("Footprint", () => {
	it("links the title to the in-app article reader by id only (no url)", async () => {
		render(Footprint, { props: { footprint: makeFootprint() } });
		const link = page.getByTestId("footprint-link");
		await expect.element(link).toBeInTheDocument();
		// id-only: the article id (after the `article:` prefix), no `?url=`.
		await expect
			.element(link)
			.toHaveAttribute(
				"href",
				"/articles/11111111-2222-3333-4444-555555555555",
			);
	});

	it("renders a plain title (no link) for non-article items", async () => {
		render(Footprint, {
			props: { footprint: makeFootprint({ itemKey: "digest:2026-06-11" }) },
		});
		expect(page.getByTestId("footprint-link").elements()).toHaveLength(0);
	});

	// D24: repeated contacts collapse server-side into one row with a count.
	it("shows a visit count when contacts are collapsed", async () => {
		render(Footprint, {
			props: {
				footprint: makeFootprint({
					contactCount: 3,
					firstOccurredAt: "2026-06-01T08:00:00Z",
				}),
			},
		});
		const count = page.getByTestId("footprint-count");
		await expect.element(count).toBeInTheDocument();
		await expect.element(count).toHaveTextContent("3");
	});

	it("shows no visit count for a single contact", async () => {
		render(Footprint, { props: { footprint: makeFootprint() } });
		expect(page.getByTestId("footprint-count").elements()).toHaveLength(0);
	});
});
