import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
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
	MAX_RETRY_ATTEMPTS,
} from "./reading-events";

const UUIDV7_RE =
	/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const testArticleUuid = (n: number): string =>
	`01938e82-7c00-7a7b-9b10-${String(n).padStart(12, "0")}`;

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
		queue.enqueueReachedEnd(testArticleUuid(1), ReadingSurface.GALLERY);
		queue.enqueueSessionEnded({
			articleId: testArticleUuid(1),
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
		queue.enqueueReachedEnd(testArticleUuid(42), ReadingSurface.SWIPE);
		tick += 3000;
		queue.enqueueSessionEnded({
			articleId: testArticleUuid(42),
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
			value: { articleId: testArticleUuid(42), surface: ReadingSurface.SWIPE },
		});
		expect(batch[2]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: testArticleUuid(42),
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
			queue.enqueueReachedEnd(testArticleUuid(i + 1), ReadingSurface.GALLERY);
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
		queue.enqueueReachedEnd(testArticleUuid(7), ReadingSurface.GALLERY);

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
			.mockResolvedValueOnce(undefined)
			.mockResolvedValueOnce(undefined);

		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		queue.enqueueReachedEnd(testArticleUuid(1), ReadingSurface.SWIPE);
		const pendingFlush = queue.flush({ keepalive: false });

		queue.enqueueReachedEnd(testArticleUuid(2), ReadingSurface.SWIPE);
		rejectSend(new Error("transient failure"));
		await pendingFlush;

		// id-2 was sent in the same flush without being blocked by id-1's failure; id-1 stays for retry.
		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[1]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-2",
		]);
		expect(queue.size).toBe(1);

		await queue.flush({ keepalive: false });
		expect(send.mock.calls[2]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-1",
		]);
		expect(queue.size).toBe(0);
	});

	it("clamps numeric session metrics into valid domain bounds", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		queue.enqueueSessionEnded({
			articleId: testArticleUuid(99),
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
				articleId: testArticleUuid(99),
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
			queue.enqueueReachedEnd(testArticleUuid(i + 1), ReadingSurface.GALLERY);
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

		queue.enqueueReachedEnd(testArticleUuid(1), ReadingSurface.SWIPE);
		queue.enqueueReachedEnd(testArticleUuid(2), ReadingSurface.SWIPE);

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
			queue.enqueueReachedEnd(testArticleUuid(i + 1), ReadingSurface.GALLERY);
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
			.mockRejectedValue(
				new ConnectError("still unavailable", Code.Unavailable),
			);

		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		for (let i = 0; i < 10; i++) {
			queue.enqueueReachedEnd(testArticleUuid(i + 1), ReadingSurface.GALLERY);
		}
		const pendingFlush = queue.flush({ keepalive: false });

		for (let i = 0; i < 200; i++) {
			queue.enqueueReachedEnd(
				testArticleUuid(1000 + i),
				ReadingSurface.GALLERY,
			);
		}

		rejectSend(new ConnectError("temporary outage", Code.Unavailable));
		await pendingFlush;

		expect(queue.size).toBe(200);

		send.mockResolvedValue(undefined);
		await queue.flush({ keepalive: false });
		const sentIds = send.mock.calls
			.slice(5)
			.flatMap((call) => call[0].map((e) => e.clientEventId));
		expect(sentIds).toHaveLength(200);
		expect(sentIds[0]).toBe("id-11");
		expect(sentIds[199]).toBe("id-210");
	});

	it("enqueues opened events and invokes onReadArticleIds from flush response", async () => {
		const onReadArticleIds = vi.fn();
		const artOpenedId = testArticleUuid(501);
		const send = vi.fn(
			async (_events: ReadingEvent[], _opts: { keepalive: boolean }) => ({
				accepted: 1,
				duplicates: 0,
				readArticleIds: [artOpenedId],
			}),
		);
		const queue = createReadingEventQueue({
			send,
			onReadArticleIds,
		});

		queue.enqueueOpened(artOpenedId, ReadingSurface.SWIPE);
		expect(send).not.toHaveBeenCalled();

		await queue.flush({ keepalive: false });

		expect(send).toHaveBeenCalledTimes(1);
		const sent = send.mock.calls[0]?.[0]?.[0];
		expect(sent?.event).toMatchObject({
			case: "opened",
			value: {
				articleId: artOpenedId,
				surface: ReadingSurface.SWIPE,
			},
		});
		expect(onReadArticleIds).toHaveBeenCalledWith([artOpenedId]);
	});

	it("retains batch on Code.Unavailable and delivers readArticleIds on retry flush within cap", async () => {
		const artRetryId = testArticleUuid(502);
		const onReadArticleIds = vi.fn();
		const send = vi
			.fn<
				(
					events: ReadingEvent[],
					options: { keepalive: boolean },
				) => Promise<{
					accepted: number;
					duplicates: number;
					readArticleIds: string[];
				}>
			>()
			.mockRejectedValueOnce(
				new ConnectError(
					"transient read-status write failure",
					Code.Unavailable,
				),
			)
			.mockResolvedValueOnce({
				accepted: 0,
				duplicates: 1,
				readArticleIds: [artRetryId],
			});
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
			onReadArticleIds,
		});

		queue.enqueueOpened(artRetryId, ReadingSurface.ARTICLE_PAGE);
		await queue.flush({ keepalive: false });

		expect(queue.size).toBe(1);
		expect(queue.size).toBeLessThanOrEqual(MAX_PENDING_EVENTS);
		expect(onReadArticleIds).not.toHaveBeenCalled();

		await queue.flush({ keepalive: false });

		expect(queue.size).toBe(0);
		expect(send.mock.calls[1]?.[0]?.[0]?.clientEventId).toBe("id-1");
		expect(onReadArticleIds).toHaveBeenCalledWith([artRetryId]);
	});

	it("ignores non-UUID articleIds at the enqueue boundary for every article event type", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const queue = createReadingEventQueue({ send });

		for (const badId of ["", "   ", "test-123", "art-1", "not-a-uuid-at-all"]) {
			queue.enqueueOpened(badId, ReadingSurface.ARTICLE_PAGE);
			queue.enqueueReachedEnd(badId, ReadingSurface.GALLERY);
			queue.enqueueSessionEnded({
				articleId: badId,
				surface: ReadingSurface.SWIPE,
				activeDwellMs: 5000,
				maxScrollRatio: 0.9,
				bodyLengthChars: 1200,
				reachedEnd: true,
			});
		}

		expect(queue.size).toBe(0);
		await queue.flush({ keepalive: false });
		expect(send).not.toHaveBeenCalled();

		const validId = testArticleUuid(901);
		queue.enqueueSurfaceViewed(ReadingSurface.ARTICLE_PAGE);
		queue.enqueueOpened(validId, ReadingSurface.ARTICLE_PAGE);
		expect(queue.size).toBe(2);
	});

	it.each([
		Code.Internal,
		Code.ResourceExhausted,
		Code.DataLoss,
	])("drops a batch failing with %s after MAX_RETRY_ATTEMPTS (3) and warns once with event count only", async (code) => {
		const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
		try {
			const send = vi
				.fn<SendFn>()
				.mockRejectedValue(new ConnectError("server error", code));
			let seq = 0;
			const queue = createReadingEventQueue({
				generateId: () => `evt-${++seq}`,
				send,
			});

			expect(MAX_RETRY_ATTEMPTS).toBe(3);

			queue.enqueueOpened(
				"01938e82-7c00-7a7b-9b10-000000000001",
				ReadingSurface.ARTICLE_PAGE,
			);
			queue.enqueueSessionEnded({
				articleId: "01938e82-7c00-7a7b-9b10-000000000001",
				surface: ReadingSurface.ARTICLE_PAGE,
				activeDwellMs: 4000,
				maxScrollRatio: 0.8,
				bodyLengthChars: 1200,
				reachedEnd: true,
			});

			await queue.flush({ keepalive: false });
			expect(send).toHaveBeenCalledTimes(1);
			expect(queue.size).toBe(2);
			expect(warnSpy).not.toHaveBeenCalled();

			await queue.flush({ keepalive: false });
			expect(send).toHaveBeenCalledTimes(2);
			expect(queue.size).toBe(2);
			expect(warnSpy).not.toHaveBeenCalled();

			await queue.flush({ keepalive: false });
			expect(send).toHaveBeenCalledTimes(3);
			expect(queue.size).toBe(0);
			expect(warnSpy).toHaveBeenCalledTimes(1);
			const warnText = warnSpy.mock.calls[0]?.map(String).join(" ") ?? "";
			expect(warnText).toContain("2");
			expect(warnText).not.toContain("01938e82-7c00-7a7b-9b10-000000000001");
			expect(warnText).not.toContain("evt-");

			await queue.flush({ keepalive: false });
			expect(send).toHaveBeenCalledTimes(3);
			expect(warnSpy).toHaveBeenCalledTimes(1);
		} finally {
			warnSpy.mockRestore();
		}
	});

	it.each([
		["Unavailable", new ConnectError("offline", Code.Unavailable)],
		["DeadlineExceeded", new ConnectError("timeout", Code.DeadlineExceeded)],
		["Canceled", new ConnectError("canceled", Code.Canceled)],
		["Aborted", new ConnectError("aborted", Code.Aborted)],
		["TypeError Failed to fetch", new TypeError("Failed to fetch")],
	])("retains %s across 3+ offline flushes without consuming retry attempts and delivers all events in order on recovery", async (_label, offlineErr) => {
		let online = false;
		const deliveredIds: string[] = [];
		const send = vi.fn<SendFn>(async (events) => {
			if (!online) {
				throw offlineErr;
			}
			for (const evt of events) {
				deliveredIds.push(evt.clientEventId);
			}
		});
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `evt-${++seq}`,
			send,
		});

		queue.enqueueSessionEnded({
			articleId: testArticleUuid(1),
			surface: ReadingSurface.ARTICLE_PAGE,
			activeDwellMs: 6000,
			maxScrollRatio: 1,
			bodyLengthChars: 1200,
			reachedEnd: true,
		});
		await queue.flush({ keepalive: false });

		queue.enqueueSessionEnded({
			articleId: testArticleUuid(2),
			surface: ReadingSurface.ARTICLE_PAGE,
			activeDwellMs: 6000,
			maxScrollRatio: 1,
			bodyLengthChars: 1200,
			reachedEnd: true,
		});
		await queue.flush({ keepalive: false });

		queue.enqueueSessionEnded({
			articleId: testArticleUuid(3),
			surface: ReadingSurface.ARTICLE_PAGE,
			activeDwellMs: 6000,
			maxScrollRatio: 1,
			bodyLengthChars: 1200,
			reachedEnd: true,
		});
		await queue.flush({ keepalive: false });

		expect(queue.size).toBe(3);

		online = true;
		await queue.flush({ keepalive: false });

		expect(queue.size).toBe(0);
		expect(deliveredIds).toEqual(["evt-1", "evt-2", "evt-3"]);
	});

	it("treats createConnectTransport TypeError: Failed to fetch (Code.Unknown with TypeError cause) as unreachable and keeps events across 3+ flushes", async () => {
		let online = false;
		const fetchSpy = vi.fn(async () => {
			if (!online) {
				throw new TypeError("Failed to fetch");
			}
			return new Response(
				JSON.stringify({ accepted: 1, duplicates: 0, readArticleIds: [] }),
				{ status: 200, headers: { "Content-Type": "application/json" } },
			);
		});
		const transport = createConnectTransport({
			baseUrl: "http://127.0.0.1/api/v2",
			fetch: fetchSpy,
		});

		const errFromTransport = await transport
			.unary(
				(await import("#lib/gen/alt/reading/v1/reading_pb.js")).ReadingService
					.method.recordReadingEvents,
				undefined,
				undefined,
				undefined,
				{ events: [] },
			)
			.catch((e: unknown) => e);
		const mapped = ConnectError.from(errFromTransport);
		expect(mapped.code).toBe(Code.Unknown);
		expect(mapped.cause).toBeInstanceOf(TypeError);

		fetchSpy.mockClear();
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `evt-${++seq}`,
			getTransport: () => transport,
		});

		queue.enqueueOpened(testArticleUuid(88), ReadingSurface.ARTICLE_PAGE);
		await queue.flush({ keepalive: false });
		await queue.flush({ keepalive: false });
		await queue.flush({ keepalive: false });
		expect(queue.size).toBe(1);

		online = true;
		await queue.flush({ keepalive: false });
		expect(queue.size).toBe(0);
		expect(fetchSpy).toHaveBeenCalledTimes(4);
	});

	it("drops a batch failing with Code.Internal three times while delivering later batches", async () => {
		const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
		try {
			const deliveredIds: string[] = [];
			const send = vi.fn<SendFn>(async (events) => {
				if (events.some((e) => e.clientEventId === "id-1")) {
					throw new ConnectError("internal", Code.Internal);
				}
				for (const e of events) {
					deliveredIds.push(e.clientEventId);
				}
			});
			let seq = 0;
			const queue = createReadingEventQueue({
				generateId: () => `id-${++seq}`,
				send,
			});

			queue.enqueueOpened(testArticleUuid(1), ReadingSurface.GALLERY);
			await queue.flush({ keepalive: false });

			queue.enqueueOpened(testArticleUuid(2), ReadingSurface.GALLERY);
			await queue.flush({ keepalive: false });

			queue.enqueueOpened(testArticleUuid(3), ReadingSurface.GALLERY);
			await queue.flush({ keepalive: false });

			expect(queue.size).toBe(0);
			expect(deliveredIds).toEqual(["id-2", "id-3"]);
			expect(warnSpy).toHaveBeenCalledTimes(1);
			expect(warnSpy.mock.calls[0]?.map(String).join(" ")).toContain("1");
		} finally {
			warnSpy.mockRestore();
		}
	});

	it("does not let a retryable failure on one batch block later batches in the same flush", async () => {
		const send = vi
			.fn<SendFn>()
			.mockRejectedValueOnce(new ConnectError("transient", Code.Unavailable))
			.mockResolvedValueOnce(undefined)
			.mockResolvedValueOnce(undefined);
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		for (let i = 0; i < MAX_FLUSH_BATCH_SIZE + 3; i++) {
			queue.enqueueReachedEnd(
				`01938e82-7c00-7a7b-9b10-${String(i + 1).padStart(12, "0")}`,
				ReadingSurface.GALLERY,
			);
		}

		await queue.flush({ keepalive: false });

		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[0]?.[0]).toHaveLength(MAX_FLUSH_BATCH_SIZE);
		expect(send.mock.calls[1]?.[0]?.map((e) => e.clientEventId)).toEqual([
			`id-${MAX_FLUSH_BATCH_SIZE + 1}`,
			`id-${MAX_FLUSH_BATCH_SIZE + 2}`,
			`id-${MAX_FLUSH_BATCH_SIZE + 3}`,
		]);
		expect(queue.size).toBe(MAX_FLUSH_BATCH_SIZE);

		await queue.flush({ keepalive: false });
		expect(send).toHaveBeenCalledTimes(3);
		expect(queue.size).toBe(0);
	});

	it("retries a failed retry batch at most once per flush while sending fresh batches behind it", async () => {
		const send = vi
			.fn<SendFn>()
			.mockRejectedValueOnce(new ConnectError("fail-1", Code.Unavailable))
			.mockRejectedValueOnce(new ConnectError("fail-2", Code.Unavailable))
			.mockResolvedValueOnce(undefined)
			.mockResolvedValueOnce(undefined);
		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		queue.enqueueOpened(
			"01938e82-7c00-7a7b-9b10-000000000011",
			ReadingSurface.SWIPE,
		);
		await queue.flush({ keepalive: false });
		expect(queue.size).toBe(1);

		queue.enqueueSessionEnded({
			articleId: "01938e82-7c00-7a7b-9b10-000000000022",
			surface: ReadingSurface.SWIPE,
			activeDwellMs: 3000,
			maxScrollRatio: 0.9,
			bodyLengthChars: 1000,
			reachedEnd: true,
		});

		await queue.flush({ keepalive: false });

		expect(send).toHaveBeenCalledTimes(3);
		expect(send.mock.calls[1]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-1",
		]);
		expect(send.mock.calls[2]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-2",
		]);
		expect(queue.size).toBe(1);

		await queue.flush({ keepalive: false });
		expect(send).toHaveBeenCalledTimes(4);
		expect(queue.size).toBe(0);
	});

	it("evicts retry batches before fresh events when cap is exceeded and warns with count only", async () => {
		const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
		try {
			const send = vi
				.fn<SendFn>()
				.mockRejectedValueOnce(
					new ConnectError("unavailable", Code.Unavailable),
				)
				.mockResolvedValue(undefined);
			let seq = 0;
			const queue = createReadingEventQueue({
				generateId: () => `id-${++seq}`,
				send,
			});

			for (let i = 0; i < 5; i++) {
				queue.enqueueReachedEnd(
					`01938e82-7c00-7a7b-9b10-00000000000${i + 1}`,
					ReadingSurface.GALLERY,
				);
			}
			await queue.flush({ keepalive: false });
			expect(queue.size).toBe(5);

			for (let i = 0; i < 198; i++) {
				queue.enqueueSurfaceViewed(ReadingSurface.GALLERY);
			}

			expect(queue.size).toBe(MAX_PENDING_EVENTS);
			expect(warnSpy).toHaveBeenCalled();
			const totalWarned = warnSpy.mock.calls
				.flatMap((c) => c.map(String))
				.join(" ");
			expect(totalWarned).not.toContain("01938e82-7c00-7a7b-9b10");
			expect(totalWarned).not.toContain("id-");

			await queue.flush({ keepalive: false });
			const sentIds = send.mock.calls
				.slice(1)
				.flatMap((call) => call[0].map((e) => e.clientEventId));
			expect(sentIds).toHaveLength(MAX_PENDING_EVENTS);
			// The first 3 retry events (id-1..id-3) were evicted; id-4, id-5 and all 198 fresh events remain.
			expect(sentIds[0]).toBe("id-4");
			expect(sentIds[1]).toBe("id-5");
			expect(sentIds[2]).toBe("id-6");
			expect(sentIds[199]).toBe("id-203");
		} finally {
			warnSpy.mockRestore();
		}
	});

	it("never sends a batch twice concurrently and retains failed older batch when a keepalive flush runs in flight", async () => {
		let rejectRegular!: (err: Error) => void;
		let resolveKeepalive!: () => void;
		const send = vi
			.fn<SendFn>()
			.mockImplementationOnce(
				() =>
					new Promise<void>((_resolve, reject) => {
						rejectRegular = reject;
					}),
			)
			.mockImplementationOnce(
				() =>
					new Promise<void>((resolve) => {
						resolveKeepalive = resolve;
					}),
			)
			.mockResolvedValueOnce(undefined);

		let seq = 0;
		const queue = createReadingEventQueue({
			generateId: () => `id-${++seq}`,
			send,
		});

		queue.enqueueOpened(
			"01938e82-7c00-7a7b-9b10-0000000000a1",
			ReadingSurface.ARTICLE_PAGE,
		);
		const regularFlush = queue.flush({ keepalive: false });

		queue.enqueueSessionEnded({
			articleId: "01938e82-7c00-7a7b-9b10-0000000000a2",
			surface: ReadingSurface.ARTICLE_PAGE,
			activeDwellMs: 5000,
			maxScrollRatio: 1,
			bodyLengthChars: 2000,
			reachedEnd: true,
		});
		const keepaliveFlush = queue.flush({ keepalive: true });

		// Keepalive flush sends only id-2 immediately without re-sending in-flight id-1.
		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[0]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-1",
		]);
		expect(send.mock.calls[0]?.[1]).toEqual({ keepalive: false });
		expect(send.mock.calls[1]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-2",
		]);
		expect(send.mock.calls[1]?.[1]).toEqual({ keepalive: true });

		resolveKeepalive();
		await keepaliveFlush;

		rejectRegular(new ConnectError("regular failed", Code.Unavailable));
		await regularFlush;

		expect(queue.size).toBe(1);
		await queue.flush({ keepalive: false });
		expect(send).toHaveBeenCalledTimes(3);
		expect(send.mock.calls[2]?.[0]?.map((e) => e.clientEventId)).toEqual([
			"id-1",
		]);
		expect(queue.size).toBe(0);
	});
});
