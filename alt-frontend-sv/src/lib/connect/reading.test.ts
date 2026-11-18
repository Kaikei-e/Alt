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

let lastTransportConfig: Record<string, unknown> | null = null;
vi.mock("@connectrpc/connect-web", () => ({
	createConnectTransport: vi.fn((config: Record<string, unknown>) => {
		lastTransportConfig = config;
		return { __keepaliveTransport: true };
	}),
}));

describe("reading Connect client", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.unstubAllGlobals();
		vi.clearAllMocks();
		lastTransportConfig = null;
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

	it("builds a cached keepalive transport with credentials and keepalive flags", async () => {
		const { createConnectTransport } = await import("@connectrpc/connect-web");
		const { createKeepaliveTransport } = await import("./reading");

		const first = createKeepaliveTransport();
		const second = createKeepaliveTransport();

		expect(second).toBe(first);
		expect(createConnectTransport).toHaveBeenCalledTimes(1);
		expect(lastTransportConfig?.baseUrl).toBe("/sv/api/v2");

		const fetchFn = lastTransportConfig?.fetch as (
			input: RequestInfo | URL,
			init?: RequestInit,
		) => Promise<Response>;
		const nativeFetch = vi
			.fn()
			.mockResolvedValue(new Response(null, { status: 200 }));
		vi.stubGlobal("fetch", nativeFetch);

		await fetchFn(
			"/sv/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
			{
				method: "POST",
				body: "{}",
			},
		);

		expect(nativeFetch).toHaveBeenCalledWith(
			"/sv/api/v2/alt.reading.v1.ReadingService/RecordReadingEvents",
			expect.objectContaining({
				method: "POST",
				body: "{}",
				credentials: "include",
				keepalive: true,
			}),
		);
	});

	it("re-exports the reading client helpers from the connect barrel", async () => {
		const barrel = await import("./index");

		expect(typeof barrel.recordReadingEvents).toBe("function");
		expect(typeof barrel.createReadingClient).toBe("function");
		expect(typeof barrel.createKeepaliveTransport).toBe("function");
	});
});
