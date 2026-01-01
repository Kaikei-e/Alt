import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";

import { renderFeedFixture } from "../../../../tests/fixtures/feeds";
import FeedCard from "./FeedCard.svelte";

describe("FeedCard", () => {
	it("renders feed metadata and button when unread", async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		render(FeedCard as any, {
			props: {
				feed: renderFeedFixture,
				isReadStatus: false,
				setIsReadStatus: vi.fn(),
			},
		});

		await expect.element(page.getByText(renderFeedFixture.title)).toBeInTheDocument();
		await expect.element(page.getByText(renderFeedFixture.excerpt)).toBeInTheDocument();
		await expect.element(
			page.getByRole("button", { name: /mark .* as read/i }),
		).toBeInTheDocument();
	});

	it("calls setIsReadStatus with normalized URL when Mark as read is clicked", async () => {
		const setIsReadStatus = vi.fn();
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		render(FeedCard as any, {
			props: {
				feed: renderFeedFixture,
				isReadStatus: false,
				setIsReadStatus,
			},
		});

		const actionButton = page.getByRole("button", {
			name: /mark .* as read/i,
		});
		await actionButton.click();

		expect(setIsReadStatus).toHaveBeenCalledWith(
			renderFeedFixture.normalizedUrl,
		);
	});

	it("renders nothing when feed is already read", async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		render(FeedCard as any, {
			props: {
				feed: renderFeedFixture,
				isReadStatus: true,
				setIsReadStatus: vi.fn(),
			},
		});

		await expect.element(page.getByTestId("feed-card")).not.toBeInTheDocument();
	});
});
