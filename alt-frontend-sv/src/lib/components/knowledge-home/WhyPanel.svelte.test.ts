import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { WhyReasonData } from "$lib/connect/knowledge_home";
import WhyPanel from "./WhyPanel.svelte";

describe("WhyPanel", () => {
	it("renders a list of why reasons", async () => {
		const reasons: WhyReasonData[] = [
			{ code: "new_unread" },
			{ code: "tag_hotspot", tag: "AI" },
		];

		render(WhyPanel as never, { props: { reasons } });

		await expect.element(page.getByText("New")).toBeInTheDocument();
		await expect.element(page.getByText("Trending: AI")).toBeInTheDocument();
	});

	it("shows heading", async () => {
		render(WhyPanel as never, {
			props: { reasons: [{ code: "new_unread" }] },
		});

		await expect
			.element(page.getByText("Why this was surfaced"))
			.toBeInTheDocument();
	});

	it("shows fallback when no reasons provided", async () => {
		render(WhyPanel as never, { props: { reasons: [] } });

		await expect
			.element(page.getByText("Matched by general relevance"))
			.toBeInTheDocument();
	});

	it("categorizes source_why reasons", async () => {
		const reasons: WhyReasonData[] = [
			{ code: "new_unread" },
			{ code: "summary_completed" },
		];

		render(WhyPanel as never, { props: { reasons } });

		await expect.element(page.getByText("New")).toBeInTheDocument();
		await expect.element(page.getByText("Summarized")).toBeInTheDocument();
	});

	it("shows change_why for supersede-related reasons", async () => {
		const reasons: WhyReasonData[] = [{ code: "summary_completed" }];

		render(WhyPanel as never, { props: { reasons } });

		await expect.element(page.getByText("Summarized")).toBeInTheDocument();
	});
});
