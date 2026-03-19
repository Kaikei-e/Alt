import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "vitest-browser-svelte";
import type { WhyReasonData } from "$lib/connect/knowledge_home";
import WhyPanel from "./WhyPanel.svelte";

describe("WhyPanel", () => {
	afterEach(() => {
		cleanup();
	});

	it("renders a list of why reasons", async () => {
		const reasons: WhyReasonData[] = [
			{ code: "new_unread" },
			{ code: "tag_hotspot", tag: "AI" },
		];

		const view = render(WhyPanel as never, { props: { reasons } });

		expect(view.container.textContent).toContain("New");
		expect(view.container.textContent).toContain("Trending: AI");
	});

	it("shows heading", async () => {
		const view = render(WhyPanel as never, {
			props: { reasons: [{ code: "new_unread" }] },
		});

		await expect
			.element(view.getByText("Why this was surfaced"))
			.toBeInTheDocument();
	});

	it("shows fallback when no reasons provided", async () => {
		const view = render(WhyPanel as never, { props: { reasons: [] } });

		await expect
			.element(view.getByText("Matched by general relevance"))
			.toBeInTheDocument();
	});

	it("categorizes source_why reasons", async () => {
		const reasons: WhyReasonData[] = [
			{ code: "new_unread" },
			{ code: "summary_completed" },
		];

		const view = render(WhyPanel as never, { props: { reasons } });

		expect(view.container.textContent).toContain("New");
		expect(view.container.textContent).toContain("Summarized");
	});

	it("shows change_why for supersede-related reasons", async () => {
		const reasons: WhyReasonData[] = [{ code: "summary_completed" }];

		const view = render(WhyPanel as never, { props: { reasons } });

		expect(view.container.textContent).toContain("Summarized");
	});
});
