import {
	create,
	enumToJson,
	fromBinary,
	fromJson,
	type JsonObject,
	toBinary,
	toJson,
} from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { createConnectTransport } from "@connectrpc/connect-web";
import { describe, expect, it, vi } from "vitest";
import { recordReadingEvents } from "#lib/connect/reading.js";
import {
	ReachedEndSchema,
	ReadingEventSchema,
	ReadingSurface,
	ReadingSurfaceSchema,
	RecordReadingEventsRequestSchema,
	RecordReadingEventsResponseSchema,
	SessionEndedSchema,
	SurfaceViewedSchema,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import { createReadingEventQueue } from "#lib/reading/reading-events.js";
import { createReadingSession } from "#lib/reading/reading-session.js";

describe("Reading API Contract", () => {
	describe("ReadingSurface enum", () => {
		it("pins numeric and JSON names for every surface", () => {
			expect(ReadingSurface.UNSPECIFIED).toBe(0);
			expect(ReadingSurface.GALLERY).toBe(1);
			expect(ReadingSurface.SWIPE).toBe(2);
			expect(ReadingSurface.ARTICLE_PAGE).toBe(3);
			expect(ReadingSurface.OTHER).toBe(4);

			expect(enumToJson(ReadingSurfaceSchema, ReadingSurface.UNSPECIFIED)).toBe(
				"READING_SURFACE_UNSPECIFIED",
			);
			expect(enumToJson(ReadingSurfaceSchema, ReadingSurface.GALLERY)).toBe(
				"READING_SURFACE_GALLERY",
			);
			expect(enumToJson(ReadingSurfaceSchema, ReadingSurface.SWIPE)).toBe(
				"READING_SURFACE_SWIPE",
			);
			expect(
				enumToJson(ReadingSurfaceSchema, ReadingSurface.ARTICLE_PAGE),
			).toBe("READING_SURFACE_ARTICLE_PAGE");
			expect(enumToJson(ReadingSurfaceSchema, ReadingSurface.OTHER)).toBe(
				"READING_SURFACE_OTHER",
			);
		});
	});

	describe("RecordReadingEventsRequest JSON shape", () => {
		it("serializes surface_viewed, reached_end and session_ended oneof variants with exact field names", () => {
			const occurredAt = timestampFromDate(
				new Date("2026-11-18T04:05:06.000Z"),
			);
			const request = create(RecordReadingEventsRequestSchema, {
				events: [
					create(ReadingEventSchema, {
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000001",
						occurredAt,
						event: {
							case: "surfaceViewed",
							value: create(SurfaceViewedSchema, {
								surface: ReadingSurface.GALLERY,
							}),
						},
					}),
					create(ReadingEventSchema, {
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000002",
						occurredAt,
						event: {
							case: "reachedEnd",
							value: create(ReachedEndSchema, {
								articleId: "article-77",
								surface: ReadingSurface.SWIPE,
							}),
						},
					}),
					create(ReadingEventSchema, {
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000003",
						occurredAt,
						event: {
							case: "sessionEnded",
							value: create(SessionEndedSchema, {
								articleId: "article-77",
								surface: ReadingSurface.ARTICLE_PAGE,
								activeDwellMs: 45000n,
								maxScrollRatio: 0.92,
								bodyLengthChars: 3200,
								reachedEnd: true,
							}),
						},
					}),
				],
			});

			const json = toJson(RecordReadingEventsRequestSchema, request);

			expect(json).toEqual({
				events: [
					{
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000001",
						occurredAt: "2026-11-18T04:05:06Z",
						surfaceViewed: {
							surface: "READING_SURFACE_GALLERY",
						},
					},
					{
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000002",
						occurredAt: "2026-11-18T04:05:06Z",
						reachedEnd: {
							articleId: "article-77",
							surface: "READING_SURFACE_SWIPE",
						},
					},
					{
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000003",
						occurredAt: "2026-11-18T04:05:06Z",
						sessionEnded: {
							articleId: "article-77",
							surface: "READING_SURFACE_ARTICLE_PAGE",
							activeDwellMs: "45000",
							maxScrollRatio: 0.92,
							bodyLengthChars: 3200,
							reachedEnd: true,
						},
					},
				],
			});

			const jsonEvents = ((json as JsonObject).events ?? []) as JsonObject[];
			expect(Object.keys(jsonEvents[0] ?? {})).toEqual([
				"clientEventId",
				"occurredAt",
				"surfaceViewed",
			]);
			expect(Object.keys(jsonEvents[1] ?? {})).toEqual([
				"clientEventId",
				"occurredAt",
				"reachedEnd",
			]);
			expect(Object.keys(jsonEvents[2] ?? {})).toEqual([
				"clientEventId",
				"occurredAt",
				"sessionEnded",
			]);

			const restoredFromJson = fromJson(RecordReadingEventsRequestSchema, json);
			expect(restoredFromJson.events).toHaveLength(3);
			expect(restoredFromJson.events[2]?.event).toMatchObject({
				case: "sessionEnded",
				value: {
					articleId: "article-77",
					surface: ReadingSurface.ARTICLE_PAGE,
					activeDwellMs: 45000n,
					maxScrollRatio: 0.92,
					bodyLengthChars: 3200,
					reachedEnd: true,
				},
			});
		});

		it("round-trips through binary serialization", () => {
			const original = create(RecordReadingEventsRequestSchema, {
				events: [
					create(ReadingEventSchema, {
						clientEventId: "01938e82-7c00-7a7b-9b10-000000000010",
						occurredAt: timestampFromDate(new Date("2026-11-18T05:00:00.123Z")),
						event: {
							case: "sessionEnded",
							value: create(SessionEndedSchema, {
								articleId: "article-binary",
								surface: ReadingSurface.GALLERY,
								activeDwellMs: 1250n,
								maxScrollRatio: 0.5,
								bodyLengthChars: 1800,
								reachedEnd: false,
							}),
						},
					}),
				],
			});

			const restored = fromBinary(
				RecordReadingEventsRequestSchema,
				toBinary(RecordReadingEventsRequestSchema, original),
			);

			expect(restored.events).toHaveLength(1);
			expect(restored.events[0]?.clientEventId).toBe(
				"01938e82-7c00-7a7b-9b10-000000000010",
			);
			expect(restored.events[0]?.event).toMatchObject({
				case: "sessionEnded",
				value: {
					articleId: "article-binary",
					surface: ReadingSurface.GALLERY,
					activeDwellMs: 1250n,
					maxScrollRatio: 0.5,
					bodyLengthChars: 1800,
					reachedEnd: false,
				},
			});
		});
	});

	describe("Wire payload from ReadingService client", () => {
		it("sends the expected Connect JSON payload from queue and session", async () => {
			let capturedUrl = "";
			let capturedBody = "";
			const stubFetch = vi.fn(
				async (input: RequestInfo | URL, init?: RequestInit) => {
					capturedUrl = String(input);
					if (typeof init?.body === "string") {
						capturedBody = init.body;
					} else if (init?.body instanceof Uint8Array) {
						capturedBody = new TextDecoder().decode(init.body);
					}
					return new Response(JSON.stringify({ accepted: 3, duplicates: 0 }), {
						status: 200,
						headers: { "Content-Type": "application/json" },
					});
				},
			);

			const transport = createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			});

			let seq = 0;
			const ids = [
				"01938e82-7c00-7a7b-9b10-100000000001",
				"01938e82-7c00-7a7b-9b10-100000000002",
				"01938e82-7c00-7a7b-9b10-100000000003",
			];
			const queue = createReadingEventQueue({
				clock: () => new Date("2026-11-18T06:00:00.000Z"),
				generateId: () => ids[seq++] ?? "01938e82-7c00-7a7b-9b10-100000000099",
				send: (events) => recordReadingEvents(transport, events),
			});

			queue.enqueueSurfaceViewed(ReadingSurface.SWIPE);
			const session = createReadingSession({
				articleId: "article-wire",
				surface: ReadingSurface.SWIPE,
				bodyLengthChars: 2500,
				queue,
				dwell: { stop: () => 18400 },
			});
			session.onScroll(0.96);
			session.markReachedEnd();
			session.end();

			await queue.flush({ keepalive: false });

			expect(capturedUrl).toBe(
				"http://127.0.0.1/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
			);
			expect(JSON.parse(capturedBody)).toEqual({
				events: [
					{
						clientEventId: "01938e82-7c00-7a7b-9b10-100000000001",
						occurredAt: "2026-11-18T06:00:00Z",
						surfaceViewed: {
							surface: "READING_SURFACE_SWIPE",
						},
					},
					{
						clientEventId: "01938e82-7c00-7a7b-9b10-100000000002",
						occurredAt: "2026-11-18T06:00:00Z",
						reachedEnd: {
							articleId: "article-wire",
							surface: "READING_SURFACE_SWIPE",
						},
					},
					{
						clientEventId: "01938e82-7c00-7a7b-9b10-100000000003",
						occurredAt: "2026-11-18T06:00:00Z",
						sessionEnded: {
							articleId: "article-wire",
							surface: "READING_SURFACE_SWIPE",
							activeDwellMs: "18400",
							maxScrollRatio: 0.96,
							bodyLengthChars: 2500,
							reachedEnd: true,
						},
					},
				],
			});
		});

		it("excludes titles, URLs and text fields from every event schema", () => {
			const forbiddenFields = new Set([
				"title",
				"url",
				"text",
				"body",
				"content",
				"excerpt",
				"summary",
			]);

			for (const schema of [
				ReadingEventSchema,
				SurfaceViewedSchema,
				ReachedEndSchema,
				SessionEndedSchema,
			]) {
				for (const field of schema.fields) {
					expect(forbiddenFields.has(field.localName)).toBe(false);
					expect(forbiddenFields.has(field.name)).toBe(false);
				}
			}
		});
	});

	describe("RecordReadingEventsResponse", () => {
		it("carries accepted and duplicates counts and defaults both to zero", () => {
			const empty = create(RecordReadingEventsResponseSchema, {});
			expect(empty.accepted).toBe(0);
			expect(empty.duplicates).toBe(0);

			const populated = create(RecordReadingEventsResponseSchema, {
				accepted: 12,
				duplicates: 3,
			});
			expect(toJson(RecordReadingEventsResponseSchema, populated)).toEqual({
				accepted: 12,
				duplicates: 3,
			});
		});
	});
});
