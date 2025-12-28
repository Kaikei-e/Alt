import { describe, expect, test, vi } from "vitest";
import { parseSSEStream, processAugurStreamingText } from "./streamingRenderer";

function createMockReader(chunks: string[]) {
	const encoder = new TextEncoder();
	const stream = new ReadableStream<Uint8Array>({
		start(controller) {
			for (const chunk of chunks) {
				controller.enqueue(encoder.encode(chunk));
			}
			controller.close();
		},
	});
	return stream.getReader();
}

describe("parseSSEStream", () => {
	test("should parse simple data events", async () => {
		const chunks = ["data: hello\n\n", "data: world\n\n"];
		const reader = createMockReader(chunks);
		const events = [];
		for await (const event of parseSSEStream(reader)) {
			events.push(event);
		}
		expect(events).toHaveLength(2);
		expect(events[0]).toEqual({ event: "message", data: "hello" });
		expect(events[1]).toEqual({ event: "message", data: "world" });
	});

	test("should parse custom events", async () => {
		const chunks = ["event: delta\ndata: hi\n\n", "event: meta\ndata: {}\n\n"];
		const reader = createMockReader(chunks);
		const events = [];
		for await (const event of parseSSEStream(reader)) {
			events.push(event);
		}
		expect(events).toHaveLength(2);
		expect(events[0]).toEqual({ event: "delta", data: "hi" });
		expect(events[1]).toEqual({ event: "meta", data: "{}" });
	});

	test("should handle split chunks", async () => {
		const chunks = ["event: delta\n", "data: split", "\n\n"];
		const reader = createMockReader(chunks);
		const events = [];
		for await (const event of parseSSEStream(reader)) {
			events.push(event);
		}
		expect(events).toHaveLength(1);
		expect(events[0]).toEqual({ event: "delta", data: "split" });
	});
});

describe("processAugurStreamingText", () => {
	test("should handle delta events", async () => {
		const updateState = vi.fn();
		const reader = createMockReader(["event: delta\ndata: chunks\n\n"]);

		await processAugurStreamingText(reader, updateState);

		expect(updateState).toHaveBeenCalledWith("chunks");
	});

	test("should handle meta events", async () => {
		const updateState = vi.fn();
		const onMetadata = vi.fn();
		const reader = createMockReader(['event: meta\ndata: {"foo": "bar"}\n\n']);

		await processAugurStreamingText(reader, updateState, { onMetadata });

		expect(onMetadata).toHaveBeenCalledWith({ foo: "bar" });
	});

	test("should handle fallback events", async () => {
		const updateState = vi.fn();
		const onMetadata = vi.fn();
		const reader = createMockReader([
			"event: fallback\ndata: insufficient\n\n",
		]);

		await processAugurStreamingText(reader, updateState, { onMetadata });

		expect(onMetadata).toHaveBeenCalledWith({
			fallback: true,
			code: "insufficient",
		});
	});
});
