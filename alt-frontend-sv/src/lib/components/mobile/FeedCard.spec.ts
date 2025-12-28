import { render, screen } from "@testing-library/svelte/svelte5";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { renderFeedFixture } from "../../../../tests/fixtures/feeds";
import FeedCard from "./FeedCard.svelte";

describe("FeedCard", () => {
	it("renders feed metadata and button when unread", () => {
		render(FeedCard, {
			props: {
				feed: renderFeedFixture,
				isReadStatus: false,
				setIsReadStatus: vi.fn(),
			},
		});

		expect(screen.getByText(renderFeedFixture.title)).toBeInTheDocument();
		expect(screen.getByText(renderFeedFixture.excerpt)).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: /mark .* as read/i }),
		).toBeInTheDocument();
	});

	it("calls setIsReadStatus with normalized URL when Mark as read is clicked", async () => {
		const setIsReadStatus = vi.fn();
		render(FeedCard, {
			props: {
				feed: renderFeedFixture,
				isReadStatus: false,
				setIsReadStatus,
			},
		});

		const actionButton = screen.getByRole("button", {
			name: /mark .* as read/i,
		});
		await userEvent.click(actionButton);

		expect(setIsReadStatus).toHaveBeenCalledWith(
			renderFeedFixture.normalizedUrl,
		);
	});

	it("renders nothing when feed is already read", () => {
		render(FeedCard, {
			props: {
				feed: renderFeedFixture,
				isReadStatus: true,
				setIsReadStatus: vi.fn(),
			},
		});

		expect(screen.queryByTestId("feed-card")).toBeNull();
	});
});
