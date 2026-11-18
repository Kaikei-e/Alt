import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	ReadingEventSchema,
	ReadingSurface,
	SurfaceViewedSchema,
} from "#lib/gen/alt/reading/v1/reading_pb.js";

vi.mock("$app/paths", () => ({
	resolve: (path: string) => `/sv/${path.replace(/^\//, "")}`,
}));

const mockRecordReadingEvents = vi.fn();

vi.mock("@connectrpc/connect", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@connectrpc/connect")>();
	return {
		...actual,
		createClient: vi.fn(() => ({
			recordReadingEvents: mockRecordReadingEvents,
		})),
	};
});

describe("reading Connect client", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.clearAllMocks();
	});

	it("creates a ReadingService client bound to the provided transport", async () => {
		const { createClient } = await import("@connectrpc/connect");
		const { ReadingService } = await import(
			"#lib/gen/alt/reading/v1/reading_pb.js"
		);
		const { createReadingClient } = await import("./reading");

		const transport = {} as Parameters<typeof createReadingClient>[0];
		createReadingClient(transport);

		expect(createClient).toHaveBeenCalledWith(ReadingService, transport);
	});

	it("sends reading events and maps accepted and duplicates counts", async () => {
		mockRecordReadingEvents.mockResolvedValue({
			accepted: 2,
			duplicates: 1,
		});
		const { recordReadingEvents } = await import("./reading");
		const transport = {} as Parameters<typeof recordReadingEvents>[0];

		const event = create(ReadingEventSchema, {
			clientEventId: "01938e82-7c00-7a7b-9b10-0123456789ab",
			occurredAt: timestampFromDate(new Date("2026-11-18T00:00:00Z")),
			event: {
				case: "surfaceViewed",
				value: create(SurfaceViewedSchema, {
					surface: ReadingSurface.GALLERY,
				}),
			},
		});

		const result = await recordReadingEvents(transport, [event]);

		expect(mockRecordReadingEvents).toHaveBeenCalledWith({
			events: [event],
		});
		expect(result).toEqual({ accepted: 2, duplicates: 1 });
	});

	it("re-exports the reading client helpers from the connect barrel", async () => {
		const barrel = await import("./index");

		expect(typeof barrel.recordReadingEvents).toBe("function");
		expect(typeof barrel.createReadingClient).toBe("function");
		expect(typeof barrel.createKeepaliveTransport).toBe("function");
	});
});
