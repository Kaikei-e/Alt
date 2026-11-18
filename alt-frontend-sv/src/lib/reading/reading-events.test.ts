import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, type Transport } from "@connectrpc/connect";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	type ReadingEvent,
	ReadingSurface,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import {
	createReadingEventQueue,
	type FlushOptions,
	MAX_FLUSH_BATCH_SIZE,
	MAX_KEEPALIVE_BATCH_SIZE,
	MAX_PENDING_EVENTS,
} from "./reading-events";

const UUIDV7_RE =
	/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

type SendFn = (
	events: ReadingEvent[],
	options: { keepalive: boolean },
) => Promise<void>;

function isoOccurredAt(event: ReadingEvent | undefined): string | undefined {
	return event?.occurredAt
		? timestampDate(event.occurredAt).toISOString()
		: undefined;
}

describe("createReadingEventQueue", () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it("throws at construction time when neither send nor getTransport is provided", () => {
		expect(() => createReadingEventQueue()).toThrow(
			/send.*getTransport|getTransport.*send/i,
		);
		expect(() => createReadingEventQueue({})).toThrow(
			/send.*getTransport|getTransport.*send/i,
		);
	});

	it("does not send anything on enqueue until flush is called", () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		queue.enqueueSurfaceViewed(ReadingSurface.GALLERY);
		queue.enqueueReachedEnd("article-1", ReadingSurface.GALLERY);
		queue.enqueueSessionEnded({
			articleId: "article-1",
			surface: ReadingSurface.GALLERY,
			activeDwellMs: 12000,
			maxScrollRatio: 0.9,
			bodyLengthChars: 2400,
			reachedEnd: true,
		});

		expect(send).not.toHaveBeenCalled();
		expect(queue.size).toBe(3);
	});

	it("stamps uuidv7 client_event_id and occurred_at from the injectable clock at enqueue time", async () => {
		let tick = Date.parse("2026-11-18T01:00:00.000Z");
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({
			clock: () => new Date(tick),
			send,
		});

		queue.enqueueSurfaceViewed(ReadingSurface.SWIPE);
		tick += 5000;
		queue.enqueueReachedEnd("article-42", ReadingSurface.SWIPE);
		tick += 3000;
		queue.enqueueSessionEnded({
			articleId: "article-42",
			surface: ReadingSurface.SWIPE,
			activeDwellMs: 8000,
			maxScrollRatio: 1,
			bodyLengthChars: 1500,
			reachedEnd: true,
		});

		tick += 60000;
		const flushOptions: FlushOptions = { keepalive: false };
		await queue.flush(flushOptions);

		expect(send).toHaveBeenCalledTimes(1);
		const firstCall = send.mock.calls[0];
		expect(firstCall?.[1]).toEqual({ keepalive: false });
		const batch = firstCall?.[0] ?? [];
		expect(batch).toHaveLength(3);

		expect(batch[0]?.clientEventId).toMatch(UUIDV7_RE);
		expect(batch[1]?.clientEventId).toMatch(UUIDV7_RE);
		expect(batch[2]?.clientEventId).toMatch(UUIDV7_RE);
		expect(new Set(batch.map((e) => e.clientEventId)).size).toBe(3);

		expect(isoOccurredAt(batch[0])).toBe("2026-11-18T01:00:00.000Z");
		expect(isoOccurredAt(batch[1])).toBe("2026-11-18T01:00:05.000Z");
		expect(isoOccurredAt(batch[2])).toBe("2026-11-18T01:00:08.000Z");

		expect(batch[0]?.event).toMatchObject({
			case: "surfaceViewed",
			value: { surface: ReadingSurface.SWIPE },
		});
		expect(batch[1]?.event).toMatchObject({
			case: "reachedEnd",
			value: { articleId: "article-42", surface: ReadingSurface.SWIPE },
		});
		expect(batch[2]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "article-42",
				surface: ReadingSurface.SWIPE,
				activeDwellMs: 8000n,
				maxScrollRatio: 1,
				bodyLengthChars: 1500,
				reachedEnd: true,
			},
		});
		expect(queue.size).toBe(0);
	});

	it("does nothing when flushing an empty queue", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		await queue.flush({ keepalive: false });
		await queue.flush({ keepalive: true });

		expect(send).not.toHaveBeenCalled();
	});

	it("caps standard flush requests at 50 events per request", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		for (let i = 0; i < 120; i++) {
			queue.enqueueReachedEnd(`article-${i}`, ReadingSurface.GALLERY);
		}

		await queue.flush({ keepalive: false });

		expect(MAX_FLUSH_BATCH_SIZE).toBe(50);
		expect(send).toHaveBeenCalledTimes(3);
		expect(send.mock.calls[0]?.[0]).toHaveLength(50);
		expect(send.mock.calls[1]?.[0]).toHaveLength(50);
		expect(send.mock.calls[2]?.[0]).toHaveLength(20);
		expect(queue.size).toBe(0);
	});

	it("caps keepalive flush requests at 20 events per request", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		for (let i = 0; i < 45; i++) {
			queue.enqueueSurfaceViewed(ReadingSurface.ARTICLE_PAGE);
		}

		await queue.flush({ keepalive: true });

		expect(MAX_KEEPALIVE_BATCH_SIZE).toBe(20);
		expect(send).toHaveBeenCalledTimes(3);
		expect(send.mock.calls[0]?.[0]).toHaveLength(20);
		expect(send.mock.calls[0]?.[1]).toEqual({ keepalive: true });
		expect(send.mock.calls[1]?.[0]).toHaveLength(20);
		expect(send.mock.calls[2]?.[0]).toHaveLength(5);
		expect(queue.size).toBe(0);
	});

	it("retains failed events with original ids and timestamps and never throws", async () => {
		let failNext = true;
		const send = vi.fn<SendFn>().mockImplementation(async () => {
			if (failNext) {
				throw new Error("network unavailable");
			}
		});
		const clockMs = Date.parse("2026-11-18T02:00:00.000Z");
		const queue = createReadingEventQueue({
			clock: () => clockMs,
			send,
		});

		queue.enqueueSurfaceViewed(ReadingSurface.GALLERY);
		queue.enqueueReachedEnd("article-7", ReadingSurface.GALLERY);

		await expect(queue.flush({ keepalive: false })).resolves.toBeUndefined();
		expect(queue.size).toBe(2);

		const firstAttemptIds = (send.mock.calls[0]?.[0] ?? []).map(
			(e) => e.clientEventId,
		);

		failNext = false;
		await queue.flush({ keepalive: false });

		expect(queue.size).toBe(0);
		const secondBatch = send.mock.calls[1]?.[0] ?? [];
		const secondAttemptIds = secondBatch.map((e) => e.clientEventId);
		expect(secondAttemptIds).toEqual(firstAttemptIds);
		expect(isoOccurredAt(secondBatch[0])).toBe("2026-11-18T02:00:00.000Z");
	});

	it("preserves event order when new events arrive while an in-flight flush fails", async () => {
		let rejectSend: (err: Error) => void = () => {};
		const send = vi
			.fn<SendFn>()
			.mockImplementationOnce(
				() =>
					new Promise<void>((_resolve, reject) => {
						rejectSend = reject;
					}),
			)
			.mockResolvedValueOnce(undefined);

		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		queue.enqueueReachedEnd("article-1", ReadingSurface.SWIPE);
		const pendingFlush = queue.flush({ keepalive: false });

		queue.enqueueReachedEnd("article-2", ReadingSurface.SWIPE);
		rejectSend(new Error("transient failure"));
		await pendingFlush;

		expect(queue.size).toBe(2);

		await queue.flush({ keepalive: false });
		const ids = (send.mock.calls[1]?.[0] ?? []).map((e) => e.clientEventId);
		expect(ids).toEqual(["id-1", "id-2"]);
	});

	it("clamps numeric session metrics into valid domain bounds", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		queue.enqueueSessionEnded({
			articleId: "article-bounds",
			surface: ReadingSurface.ARTICLE_PAGE,
			activeDwellMs: -50,
			maxScrollRatio: 1.4,
			bodyLengthChars: -10,
			reachedEnd: false,
		});

		await queue.flush({ keepalive: false });

		const sent = send.mock.calls[0]?.[0]?.[0];
		expect(sent?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "article-bounds",
				surface: ReadingSurface.ARTICLE_PAGE,
				activeDwellMs: 0n,
				maxScrollRatio: 1,
				bodyLengthChars: 0,
				reachedEnd: false,
			},
		});
	});

	it("selects client vs keepalive transport via getTransport when send is not overridden", async () => {
		const unary = vi.fn().mockResolvedValue({
			message: { accepted: 1, duplicates: 0 },
		});
		const standardTransport = { unary } as unknown as Transport;
		const keepaliveTransport = { unary } as unknown as Transport;
		const getTransport = vi.fn((keepalive: boolean) =>
			keepalive ? keepaliveTransport : standardTransport,
		);

		const queue = createReadingEventQueue({ getTransport });
		queue.enqueueSurfaceViewed(ReadingSurface.GALLERY);
		await queue.flush({ keepalive: false });

		queue.enqueueSurfaceViewed(ReadingSurface.GALLERY);
		await queue.flush({ keepalive: true });

		expect(getTransport).toHaveBeenNthCalledWith(1, false);
		expect(getTransport).toHaveBeenNthCalledWith(2, true);
		expect(unary).toHaveBeenCalledTimes(2);
	});

	it.each([
		Code.InvalidArgument,
		Code.PermissionDenied,
		Code.Unauthenticated,
		Code.FailedPrecondition,
		Code.Unimplemented,
		Code.NotFound,
	])("drops failed batch on permanent Connect error %s without blocking later batches", async (code) => {
		const send = vi
			.fn<SendFn>()
			.mockRejectedValueOnce(new ConnectError("permanent failure", code))
			.mockResolvedValueOnce(undefined);
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		for (let i = 0; i < MAX_FLUSH_BATCH_SIZE + 2; i++) {
			queue.enqueueReachedEnd(`article-${i}`, ReadingSurface.GALLERY);
		}

		await expect(queue.flush({ keepalive: false })).resolves.toBeUndefined();

		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[0]?.[0]).toHaveLength(MAX_FLUSH_BATCH_SIZE);
		expect(send.mock.calls[1]?.[0]).toHaveLength(2);
		expect(send.mock.calls[1]?.[0]?.map((e) => e.clientEventId)).toEqual([
			`id-${MAX_FLUSH_BATCH_SIZE + 1}`,
			`id-${MAX_FLUSH_BATCH_SIZE + 2}`,
		]);
		expect(queue.size).toBe(0);
	});

	it.each([
		Code.Unavailable,
		Code.ResourceExhausted,
		Code.Internal,
		Code.DeadlineExceeded,
	])("retains failed batch at the head on transient Connect error %s", async (code) => {
		const send = vi
			.fn<SendFn>()
			.mockRejectedValueOnce(new ConnectError("transient failure", code))
			.mockResolvedValueOnce(undefined);
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		queue.enqueueReachedEnd("article-1", ReadingSurface.SWIPE);
		queue.enqueueReachedEnd("article-2", ReadingSurface.SWIPE);

		await queue.flush({ keepalive: false });
		expect(queue.size).toBe(2);

		await queue.flush({ keepalive: false });
		expect(queue.size).toBe(0);
		expect(send.mock.calls[1]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-1",
			"id-2",
		]);
	});

	it("caps pending events at 200 by dropping the oldest events", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		expect(MAX_PENDING_EVENTS).toBe(200);

		for (let i = 0; i < 205; i++) {
			queue.enqueueReachedEnd(`article-${i}`, ReadingSurface.GALLERY);
		}

		expect(queue.size).toBe(200);

		await queue.flush({ keepalive: false });
		const sentIds = send.mock.calls.flatMap((call) =>
			call[0].map((e) => e.clientEventId),
		);
		expect(sentIds).toHaveLength(200);
		expect(sentIds[0]).toBe("id-6");
		expect(sentIds[199]).toBe("id-205");
	});

	it("enforces the 200-event cap when a failed batch returns while the queue filled up", async () => {
		let rejectSend: (err: Error) => void = () => {};
		const send = vi
			.fn<SendFn>()
			.mockImplementationOnce(
				() =>
					new Promise<void>((_resolve, reject) => {
						rejectSend = reject;
					}),
			)
			.mockResolvedValue(undefined);

		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		for (let i = 0; i < 10; i++) {
			queue.enqueueReachedEnd(`initial-${i}`, ReadingSurface.GALLERY);
		}
		const pendingFlush = queue.flush({ keepalive: false });

		for (let i = 0; i < 200; i++) {
			queue.enqueueReachedEnd(`newer-${i}`, ReadingSurface.GALLERY);
		}

		rejectSend(new ConnectError("temporary outage", Code.Unavailable));
		await pendingFlush;

		expect(queue.size).toBe(200);

		await queue.flush({ keepalive: false });
		const sentIds = send.mock.calls
			.slice(1)
			.flatMap((call) => call[0].map((e) => e.clientEventId));
		expect(sentIds).toHaveLength(200);
		expect(sentIds[0]).toBe("id-11");
		expect(sentIds[199]).toBe("id-210");
	});
});
