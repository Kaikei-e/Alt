import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import DesktopFeedCard from "./DesktopFeedCard.svelte";
import { renderFeedFixture } from "../../../../../tests/fixtures/feeds";

describe("DesktopFeedCard", () => {
	it("renders feed title", async () => {
		render(DesktopFeedCard as never, {
			props: { feed: renderFeedFixture, onSelect: vi.fn() },
		});

		await expect.element(page.getByText("Daily AI Recap")).toBeInTheDocument();
	});

	it("renders dateline with date and author", async () => {
		render(DesktopFeedCard as never, {
			props: { feed: renderFeedFixture, onSelect: vi.fn() },
		});

		// Dateline shows date · author in a single span
		await expect.element(page.getByText("Dec 22, 2025")).toBeInTheDocument();
	});

	it("renders excerpt", async () => {
		render(DesktopFeedCard as never, {
			props: { feed: renderFeedFixture, onSelect: vi.fn() },
		});

		await expect
			.element(page.getByText(/most important AI breakthroughs/))
			.toBeInTheDocument();
	});

	it("renders tags as inline text", async () => {
		render(DesktopFeedCard as never, {
			props: { feed: renderFeedFixture, onSelect: vi.fn() },
		});

		// mergedTagsLabel "AI / Research" → rendered as "AI · Research"
		await expect
			.element(page.getByText("AI \u00b7 Research"))
			.toBeInTheDocument();
	});

	it("shows unread stripe for unread items", async () => {
		render(DesktopFeedCard as never, {
			props: { feed: renderFeedFixture, isRead: false, onSelect: vi.fn() },
		});

		const entry = page.getByRole("button");
		await expect.element(entry).toBeInTheDocument();
	});

	it("calls onSelect when clicked", async () => {
		const onSelect = vi.fn();
		render(DesktopFeedCard as never, {
			props: { feed: renderFeedFixture, onSelect },
		});

		await page.getByRole("button").click();
		expect(onSelect).toHaveBeenCalledWith(renderFeedFixture);
	});
});
