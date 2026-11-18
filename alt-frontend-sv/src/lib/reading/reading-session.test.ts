import { describe, expect, it, vi } from "vitest";
import {
	type ReadingEvent,
	ReadingSurface,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import { createDwellTracker } from "./dwell-tracker";
import { createReadingEventQueue } from "./reading-events";
import { createReadingSession } from "./reading-session";

function makeMocks(dwellMs = 3500) {
	return {
		queue: {
			enqueueReachedEnd: vi.fn(),
			enqueueSessionEnded: vi.fn(),
		},
		dwell: {
			stop: vi.fn(() => dwellMs),
		},
	};
}

describe("createReadingSession", () => {
	it("throws when articleId is empty or whitespace", () => {
		const { queue, dwell } = makeMocks();

		expect(() =>
			createReadingSession({
				articleId: "",
				surface: ReadingSurface.GALLERY,
				bodyLengthChars: 1200,
				queue,
				dwell,
			}),
		).toThrow();

		expect(() =>
			createReadingSession({
				articleId: "   ",
				surface: ReadingSurface.SWIPE,
				bodyLengthChars: 1200,
				queue,
				dwell,
			}),
		).toThrow();

		expect(queue.enqueueReachedEnd).not.toHaveBeenCalled();
		expect(queue.enqueueSessionEnded).not.toHaveBeenCalled();
		expect(dwell.stop).not.toHaveBeenCalled();
	});

	it("tracks the maximum scroll ratio clamped to 0..1 without per-scroll enqueues", () => {
		const { queue, dwell } = makeMocks(5200);
		const session = createReadingSession({
			articleId: "article-100",
			surface: ReadingSurface.ARTICLE_PAGE,
			bodyLengthChars: 4800,
			queue,
			dwell,
		});

		session.onScroll(-0.25);
		session.onScroll(0.35);
		session.onScroll(0.75);
		session.onScroll(0.4);
		session.onScroll(Number.NaN);

		expect(queue.enqueueReachedEnd).not.toHaveBeenCalled();
		expect(queue.enqueueSessionEnded).not.toHaveBeenCalled();

		session.end();

		expect(queue.enqueueSessionEnded).toHaveBeenCalledTimes(1);
		expect(queue.enqueueSessionEnded).toHaveBeenCalledWith({
			articleId: "article-100",
			surface: ReadingSurface.ARTICLE_PAGE,
			activeDwellMs: 5200,
			maxScrollRatio: 0.75,
			bodyLengthChars: 4800,
			reachedEnd: false,
		});
	});

	it("clamps scroll ratios above 1 down to 1", () => {
		const { queue, dwell } = makeMocks(1000);
		const session = createReadingSession({
			articleId: "article-overscroll",
			surface: ReadingSurface.GALLERY,
			bodyLengthChars: 900,
			queue,
			dwell,
		});

		session.onScroll(1.3);
		session.end();

		expect(queue.enqueueSessionEnded).toHaveBeenCalledWith(
			expect.objectContaining({
				maxScrollRatio: 1,
			}),
		);
	});

	it("enqueues reached_end at most once and records reachedEnd on session_ended", () => {
		const { queue, dwell } = makeMocks(9100);
		const session = createReadingSession({
			articleId: "article-end",
			surface: ReadingSurface.SWIPE,
			bodyLengthChars: 2100,
			queue,
			dwell,
		});

		session.markReachedEnd();
		session.markReachedEnd();
		session.markReachedEnd();

		expect(queue.enqueueReachedEnd).toHaveBeenCalledTimes(1);
		expect(queue.enqueueReachedEnd).toHaveBeenCalledWith(
			"article-end",
			ReadingSurface.SWIPE,
		);

		session.end();

		expect(queue.enqueueSessionEnded).toHaveBeenCalledWith({
			articleId: "article-end",
			surface: ReadingSurface.SWIPE,
			activeDwellMs: 9100,
			maxScrollRatio: 0,
			bodyLengthChars: 2100,
			reachedEnd: true,
		});
	});

	it("makes end idempotent and ignores interactions after end", () => {
		const { queue, dwell } = makeMocks(2400);
		const session = createReadingSession({
			articleId: "article-once",
			surface: ReadingSurface.GALLERY,
			bodyLengthChars: 1600,
			queue,
			dwell,
		});

		session.onScroll(0.5);
		session.end();
		session.onScroll(0.95);
		session.markReachedEnd();
		session.end();

		expect(dwell.stop).toHaveBeenCalledTimes(1);
		expect(queue.enqueueReachedEnd).not.toHaveBeenCalled();
		expect(queue.enqueueSessionEnded).toHaveBeenCalledTimes(1);
		expect(queue.enqueueSessionEnded).toHaveBeenCalledWith({
			articleId: "article-once",
			surface: ReadingSurface.GALLERY,
			activeDwellMs: 2400,
			maxScrollRatio: 0.5,
			bodyLengthChars: 1600,
			reachedEnd: false,
		});
	});

	it("integrates with createDwellTracker and createReadingEventQueue", async () => {
		let now = 1000;
		const send = vi
			.fn<(events: ReadingEvent[]) => Promise<void>>()
			.mockResolvedValue(undefined);
		const queue = createReadingEventQueue({
			clock: () => new Date("2026-11-18T03:00:00Z"),
			send,
		});
		const dwell = createDwellTracker({
			now: () => now,
			visibility: null,
		});

		const session = createReadingSession({
			articleId: "article-integrated",
			surface: ReadingSurface.GALLERY,
			bodyLengthChars: 3300,
			queue,
			dwell,
		});

		now += 6400;
		session.onScroll(0.88);
		session.markReachedEnd();
		session.end();

		expect(send).not.toHaveBeenCalled();
		await queue.flush({ keepalive: false });

		expect(send).toHaveBeenCalledTimes(1);
		const events = send.mock.calls[0]?.[0] ?? [];
		expect(events).toHaveLength(2);
		expect(events[0]?.event.case).toBe("reachedEnd");
		expect(events[1]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "article-integrated",
				surface: ReadingSurface.GALLERY,
				activeDwellMs: 6400n,
				maxScrollRatio: 0.88,
				bodyLengthChars: 3300,
				reachedEnd: true,
			},
		});
	});
});
