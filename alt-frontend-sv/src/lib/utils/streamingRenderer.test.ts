import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { simulateTypewriterEffect } from "./streamingRenderer";

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
});
