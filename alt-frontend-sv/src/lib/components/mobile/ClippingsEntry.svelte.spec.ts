import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";

import { renderFeedFixture } from "../../../../tests/fixtures/feeds";
import ClippingsEntry from "./ClippingsEntry.svelte";

const readFeed = { ...renderFeedFixture, isRead: true };
const unreadFeed = { ...renderFeedFixture, isRead: false };

describe("ClippingsEntry", () => {
	it("renders feed title, excerpt, and author", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		await expect
			.element(page.getByText(renderFeedFixture.title))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(renderFeedFixture.excerpt))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					`${renderFeedFixture.publishedAtFormatted} · ${renderFeedFixture.author}`,
				),
			)
			.toBeInTheDocument();
	});

	it("has data-role clippings-entry attribute", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		await expect
			.element(page.getByRole("article"))
			.toHaveAttribute("data-role", "clippings-entry");
	});

	it("shows Read label when feed.isRead is true", async () => {
		render(ClippingsEntry as never, { props: { feed: readFeed } });

		await expect.element(page.getByText("Read")).toBeInTheDocument();
	});

	it("does not show Read label when feed.isRead is false", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		await expect.element(page.getByText("Read")).not.toBeInTheDocument();
	});

	it("has a Details button", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		await expect
			.element(page.getByRole("button", { name: /details/i }))
			.toBeInTheDocument();
	});

	it("has an Open link pointing to normalizedUrl", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		const openLink = page.getByRole("link", { name: /open/i });
		await expect.element(openLink).toBeInTheDocument();
		await expect
			.element(openLink)
			.toHaveAttribute("href", renderFeedFixture.normalizedUrl);
		await expect.element(openLink).toHaveAttribute("target", "_blank");
	});

	it("shows Remove button and calls onRemove when clicked", async () => {
		const onRemove = vi.fn();
		render(ClippingsEntry as never, {
			props: { feed: unreadFeed, onRemove },
		});

		const removeBtn = page.getByRole("button", { name: /remove/i });
		await expect.element(removeBtn).toBeInTheDocument();
		await removeBtn.click();

		expect(onRemove).toHaveBeenCalledWith(renderFeedFixture.normalizedUrl);
	});

	it("hides Remove button when onRemove is not provided", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		await expect
			.element(page.getByRole("button", { name: /remove/i }))
			.not.toBeInTheDocument();
	});

	it("does not use glassmorphism styling", async () => {
		render(ClippingsEntry as never, { props: { feed: unreadFeed } });

		const article = page.getByRole("article");
		await expect.element(article).toBeInTheDocument();
		const el = article.element() as HTMLElement;
		const style = window.getComputedStyle(el);
		expect(style.borderRadius).toBe("0px");
	});
});
