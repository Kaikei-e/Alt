import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";

import { renderFeedFixture } from "../../../../tests/fixtures/feeds";
import FavoriteCard from "./FavoriteCard.svelte";

const readFeed = { ...renderFeedFixture, isRead: true };
const unreadFeed = { ...renderFeedFixture, isRead: false };

describe("FavoriteCard", () => {
	it("renders feed metadata (title, excerpt, author)", async () => {
		render(FavoriteCard as never, {
			props: { feed: unreadFeed },
		});

		await expect
			.element(page.getByText(renderFeedFixture.title))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(renderFeedFixture.excerpt))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(`by ${renderFeedFixture.author}`))
			.toBeInTheDocument();
	});

	it("always renders when feed is read (unlike FeedCard which hides)", async () => {
		render(FavoriteCard as never, {
			props: { feed: readFeed },
		});

		await expect.element(page.getByTestId("favorite-card")).toBeInTheDocument();
		await expect
			.element(page.getByText(renderFeedFixture.title))
			.toBeInTheDocument();
	});

	it("shows Read badge when feed.isRead is true", async () => {
		render(FavoriteCard as never, {
			props: { feed: readFeed },
		});

		await expect.element(page.getByText("Read")).toBeInTheDocument();
	});

	it("does not show Read badge when feed.isRead is false", async () => {
		render(FavoriteCard as never, {
			props: { feed: unreadFeed },
		});

		await expect.element(page.getByText("Read")).not.toBeInTheDocument();
	});

	it("does NOT have a Mark as read button", async () => {
		render(FavoriteCard as never, {
			props: { feed: unreadFeed },
		});

		await expect
			.element(page.getByRole("button", { name: /mark .* as read/i }))
			.not.toBeInTheDocument();
	});

	it("has a Show Details button", async () => {
		render(FavoriteCard as never, {
			props: { feed: unreadFeed },
		});

		await expect.element(page.getByText("Details")).toBeInTheDocument();
	});

	it("has an Open link pointing to normalizedUrl", async () => {
		render(FavoriteCard as never, {
			props: { feed: unreadFeed },
		});

		const openLink = page.getByRole("link", { name: "Open article" });
		await expect.element(openLink).toBeInTheDocument();
		await expect
			.element(openLink)
			.toHaveAttribute("href", renderFeedFixture.normalizedUrl);
		await expect.element(openLink).toHaveAttribute("target", "_blank");
	});

	it("shows Remove button and calls onRemove when clicked", async () => {
		const onRemove = vi.fn();
		render(FavoriteCard as never, {
			props: { feed: unreadFeed, onRemove },
		});

		const removeBtn = page.getByRole("button", { name: /remove/i });
		await expect.element(removeBtn).toBeInTheDocument();
		await removeBtn.click();

		expect(onRemove).toHaveBeenCalledWith(renderFeedFixture.normalizedUrl);
	});

	it("hides Remove button when onRemove is not provided", async () => {
		render(FavoriteCard as never, {
			props: { feed: unreadFeed },
		});

		await expect
			.element(page.getByRole("button", { name: /remove/i }))
			.not.toBeInTheDocument();
	});

	it("uses muted styling for read feeds", async () => {
		render(FavoriteCard as never, {
			props: { feed: readFeed },
		});

		const title = page.getByText(renderFeedFixture.title);
		await expect.element(title).toBeInTheDocument();
		// Read feeds should not have font-semibold (muted styling)
		await expect.element(title).not.toHaveClass("font-semibold");
	});
});
