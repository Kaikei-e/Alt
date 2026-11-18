import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { recordReadingEvents } from "#lib/connect/reading.js";
import {
	flushReadingEvents,
	ReadingSurface,
} from "#lib/reading/instrumentation.js";

vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/api/client/index.js", async (importOriginal) => {
	const actual =
		await importOriginal<typeof import("#lib/api/client/index.js")>();
	return {
		...actual,
		batchPrefetchImagesClient: vi.fn().mockResolvedValue([]),
		getFeedContentOnTheFlyClient: vi
			.fn()
			.mockResolvedValue({ content: "", article_id: "" }),
		getFeedsWithCursorClient: vi
			.fn()
			.mockResolvedValue({ data: [], next_cursor: null }),
		getReadFeedsWithCursorClient: vi
			.fn()
			.mockResolvedValue({ data: [], next_cursor: null }),
		listSubscriptionsClient: vi.fn().mockResolvedValue([]),
		updateFeedReadStatusClient: vi.fn().mockResolvedValue(undefined),
	};
});

import Page from "./+page.svelte";

describe("Swipe visual preview page reading telemetry", () => {
	beforeEach(async () => {
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 0,
			duplicates: 0,
		});
		await flushReadingEvents();
		vi.mocked(recordReadingEvents).mockReset();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 1,
			duplicates: 0,
		});
	});

	it("enqueues one swipe surface_viewed event on mount", async () => {
		await page.viewport(393, 851);
		render(Page, {
			props: {
				data: {
					initialFeeds: [],
					nextCursor: null,
					articleData: {
						firstArticleImageUrl: null,
						firstArticleContent: null,
						firstArticleId: null,
					},
				},
			},
		});

		expect(recordReadingEvents).not.toHaveBeenCalled();

		await flushReadingEvents();

		expect(recordReadingEvents).toHaveBeenCalledTimes(1);
		const events = vi.mocked(recordReadingEvents).mock.calls[0]?.[1] ?? [];
		expect(events).toHaveLength(1);
		expect(events[0]?.event).toMatchObject({
			case: "surfaceViewed",
			value: { surface: ReadingSurface.SWIPE },
		});
	});
});
