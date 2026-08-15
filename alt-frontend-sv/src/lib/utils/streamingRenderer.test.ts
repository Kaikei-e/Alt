import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import {
	createStreamingRenderer,
	simulateTypewriterEffect,
} from "./streamingRenderer";

describe("simulateTypewriterEffect", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	test("emits one character per configured delay, not all at once", async () => {
		const chars: string[] = [];
		const delay = 10;
		const tick = vi.fn(async () => {});

		const typewriter = simulateTypewriterEffect((char) => chars.push(char), {
			delay,
			tick,
		});

		typewriter.add("Hi");

		// First character is emitted on the initial microtask tick, before any
		// timer-driven delay has elapsed.
		await vi.advanceTimersByTimeAsync(0);
		expect(chars).toEqual(["H"]);

		// The second character must wait for the full inter-character delay —
		// advancing by less than `delay` must not release it.
		await vi.advanceTimersByTimeAsync(delay - 1);
		expect(chars).toEqual(["H"]);

		await vi.advanceTimersByTimeAsync(1);
		expect(chars).toEqual(["H", "i"]);

		await typewriter.getPromise();
		expect(tick).toHaveBeenCalled();
	});

	test("interleaves characters from sequential add() calls in call order", async () => {
		const chars: string[] = [];
		const typewriter = simulateTypewriterEffect((char) => chars.push(char), {
			delay: 5,
		});

		// Fire both immediately (non-blocking) — the second add() must queue
		// behind the first rather than racing it.
		typewriter.add("Hi");
		typewriter.add("There");

		await vi.advanceTimersByTimeAsync(1000);
		await typewriter.getPromise();

		expect(chars.join("")).toBe("HiThere");
	});

	test("stops emitting the instant cancel() is called mid-stream", async () => {
		const chars: string[] = [];
		const typewriter = simulateTypewriterEffect((char) => chars.push(char), {
			delay: 20,
		});

		typewriter.add("LongText");

		// Deterministically release exactly two characters, then cancel —
		// no reliance on wall-clock race timing.
		await vi.advanceTimersByTimeAsync(0); // "L"
		await vi.advanceTimersByTimeAsync(20); // "o"
		typewriter.cancel();

		// Draining any further pending timers must not emit more characters.
		await vi.advanceTimersByTimeAsync(1000);
		await typewriter.getPromise();

		expect(chars).toEqual(["L", "o"]);
	});

	test("flushRemaining() emits every queued-but-untyped character at once", async () => {
		const emitted: string[] = [];
		const typewriter = simulateTypewriterEffect((text) => emitted.push(text), {
			delay: 20,
		});

		typewriter.add("LongText");

		await vi.advanceTimersByTimeAsync(0); // "L"
		await vi.advanceTimersByTimeAsync(20); // "o"

		typewriter.flushRemaining();

		expect(emitted.join("")).toBe("LongText");
	});

	test("flushRemaining() emits nothing more once the backlog is drained", async () => {
		const emitted: string[] = [];
		const typewriter = simulateTypewriterEffect((text) => emitted.push(text), {
			delay: 20,
		});

		typewriter.add("LongText");
		await vi.advanceTimersByTimeAsync(0); // "L"
		typewriter.flushRemaining();

		// Draining the pending timers must not re-emit the flushed tail, and a
		// second flush must not duplicate it either.
		await vi.advanceTimersByTimeAsync(1000);
		typewriter.flushRemaining();
		await typewriter.getPromise();

		expect(emitted.join("")).toBe("LongText");
	});

	test("flushRemaining() is a no-op when nothing was ever queued", () => {
		const emitted: string[] = [];
		const typewriter = simulateTypewriterEffect((text) => emitted.push(text), {
			delay: 20,
		});

		typewriter.flushRemaining();

		expect(emitted).toEqual([]);
	});
});

describe("createStreamingRenderer typewriter backlog", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	test("flushPending() renders the received-but-untyped text to the UI", async () => {
		let rendered = "";
		const renderer = createStreamingRenderer(
			(text) => {
				rendered += text;
			},
			{
				typewriter: true,
				typewriterDelay: 10,
			},
		);

		await renderer.processChunk("Received before the cut");
		await vi.advanceTimersByTimeAsync(10); // only two characters typed so far

		renderer.flushPending();

		expect(rendered).toBe("Received before the cut");
	});

	test("cancel() still drops the untyped backlog", async () => {
		let rendered = "";
		const renderer = createStreamingRenderer(
			(text) => {
				rendered += text;
			},
			{
				typewriter: true,
				typewriterDelay: 10,
			},
		);

		await renderer.processChunk("Received before the cut");
		await vi.advanceTimersByTimeAsync(10);

		renderer.cancel();
		await vi.advanceTimersByTimeAsync(1000);

		expect(rendered).toBe("Re");
	});

	test("flushPending() stops later chunks from being rendered", async () => {
		let rendered = "";
		const renderer = createStreamingRenderer(
			(text) => {
				rendered += text;
			},
			{
				typewriter: true,
				typewriterDelay: 10,
			},
		);

		await renderer.processChunk("Ab");
		renderer.flushPending();
		await renderer.processChunk("Cd");
		await vi.advanceTimersByTimeAsync(1000);

		expect(rendered).toBe("Ab");
	});

	test("flushPending() is harmless without the typewriter effect", async () => {
		let rendered = "";
		const renderer = createStreamingRenderer((text) => {
			rendered += text;
		});

		// The chunk path yields through a zero-delay timer, which fake timers
		// only release on demand.
		const processed = renderer.processChunk("Whole chunk");
		await vi.advanceTimersByTimeAsync(0);
		await processed;

		renderer.flushPending();

		expect(rendered).toBe("Whole chunk");
	});
});
