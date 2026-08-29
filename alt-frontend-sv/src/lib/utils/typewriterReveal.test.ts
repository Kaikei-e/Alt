/**
 * The typewriter reveal drain — the shared engine behind the ASK Augur chat
 * (AugurChat) and the Knowledge Home ask pane (useAugurPane), per ADR-000985's
 * "one shared place for everyone" obligation.
 *
 * Everything here runs against an injected FakeScheduler: no real or faked
 * requestAnimationFrame, no timers, no DOM. Virtual frames are pumped by hand
 * so every pacing assertion is deterministic.
 */
import { describe, expect, it } from "vitest";
import {
	createTypewriterReveal,
	type RevealScheduler,
	type TypewriterRevealOptions,
} from "./typewriterReveal";

class FakeScheduler implements RevealScheduler {
	time = 0;
	requestCount = 0;
	cancelled: number[] = [];
	private nextHandle = 1;
	private pending = new Map<number, (t: number) => void>();

	now(): number {
		return this.time;
	}

	request(cb: (t: number) => void): number {
		this.requestCount++;
		const handle = this.nextHandle++;
		this.pending.set(handle, cb);
		return handle;
	}

	cancel(handle: number): void {
		this.cancelled.push(handle);
		this.pending.delete(handle);
	}

	get pendingHandles(): number {
		return this.pending.size;
	}

	/** Advance virtual time and run every frame callback that was scheduled. */
	tick(dtMs: number): void {
		this.time += dtMs;
		const due = [...this.pending.values()];
		this.pending.clear();
		for (const cb of due) {
			cb(this.time);
		}
	}
}

const FRAME_MS = 16;

function setup(
	options: Partial<
		Omit<TypewriterRevealOptions, "onReveal" | "scheduler">
	> = {},
) {
	const scheduler = new FakeScheduler();
	const emissions: { t: number; text: string }[] = [];
	const reveal = createTypewriterReveal({
		onReveal: (text) => {
			emissions.push({ t: scheduler.time, text });
		},
		scheduler,
		...options,
	});
	return { scheduler, emissions, reveal };
}

/** Per-emission growth of the revealed prefix, in characters. */
function increments(emissions: { text: string }[]): number[] {
	return emissions.map(
		(e, i) => e.text.length - (i === 0 ? 0 : emissions[i - 1]!.text.length),
	);
}

function pump(scheduler: FakeScheduler, frames: number, dtMs = FRAME_MS): void {
	for (let i = 0; i < frames; i++) {
		scheduler.tick(dtMs);
	}
}

describe("createTypewriterReveal", () => {
	it("reveals nothing before the first frame runs", () => {
		const { emissions, reveal } = setup();

		reveal.push("Hello");

		expect(emissions).toEqual([]);
		expect(reveal.revealedLength).toBe(0);
		expect(reveal.pendingLength).toBe(5);
	});

	it("reveals a strict prefix of what was pushed, never more", () => {
		const { scheduler, emissions, reveal } = setup();
		const pushed = "abcdefghij".repeat(30);

		reveal.push(pushed);
		pump(scheduler, 10);

		expect(emissions.length).toBeGreaterThan(0);
		for (const e of emissions) {
			expect(pushed.startsWith(e.text)).toBe(true);
			expect(e.text.length).toBeLessThan(pushed.length);
		}
	});

	it("crawls at the floor rate when the backlog is tiny", () => {
		// minCharsPerSecond = 24 means one character every ~42ms; quantized to
		// 16ms frames that lands on 48ms boundaries.
		const { scheduler, emissions, reveal } = setup();

		reveal.push("Hi");
		pump(scheduler, 12);

		expect(emissions.map((e) => e.text)).toEqual(["H", "Hi"]);
		const gap = emissions[1]!.t - emissions[0]!.t;
		expect(gap).toBeGreaterThanOrEqual(32);
		expect(gap).toBeLessThanOrEqual(64);
	});

	it("speeds up in proportion to the backlog", () => {
		const tiny = setup();
		tiny.reveal.push("Hi");
		pump(tiny.scheduler, 10);

		const big = setup();
		big.reveal.push("x".repeat(400));
		pump(big.scheduler, 10);

		// Ten frames into a 400-char backlog most of it is already out; the
		// same ten frames move a 2-char backlog by single characters.
		expect(big.reveal.revealedLength).toBeGreaterThan(150);
		const bigMax = Math.max(...increments(big.emissions));
		const tinyMax = Math.max(...increments(tiny.emissions));
		expect(bigMax).toBeGreaterThan(tinyMax);

		// And the whole 400 characters drain in bounded time (the decay is
		// exponential toward the floor, measured ~1.12s at 16ms frames).
		pump(big.scheduler, 90);
		expect(big.reveal.revealedLength).toBe(400);
	});

	it("never reveals more than maxCharsPerFrame in one frame", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("y".repeat(5000));
		pump(scheduler, 300);

		expect(reveal.revealedLength).toBe(5000);
		for (const inc of increments(emissions)) {
			expect(inc).toBeLessThanOrEqual(24);
		}
	});

	it("converges within bounded time of the last push", () => {
		// 200 chars decay e-fold per 220ms horizon until the 24cps floor
		// finishes the tail: measured 976ms at 16ms frames, bounded at 1200ms.
		const { scheduler, reveal } = setup();

		reveal.push("z".repeat(200));
		pump(scheduler, 75); // 1200ms of virtual time

		expect(reveal.revealedLength).toBe(200);
	});

	it("clamps a long frame gap so a backgrounded tab does not dump the answer", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("w".repeat(5000));
		scheduler.tick(5000);

		expect(emissions).toHaveLength(1);
		expect(emissions[0]!.text.length).toBeLessThanOrEqual(24);
	});

	it("stops scheduling once the backlog is drained", () => {
		const { scheduler, reveal } = setup();

		reveal.push("Hi");
		pump(scheduler, 12);

		expect(reveal.revealedLength).toBe(2);
		expect(scheduler.pendingHandles).toBe(0);
	});

	it("restarts the loop when new text arrives after going idle", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("Hi");
		pump(scheduler, 12);
		expect(scheduler.pendingHandles).toBe(0);

		reveal.push(" again");
		expect(scheduler.pendingHandles).toBe(1);
		pump(scheduler, 30);

		expect(emissions.at(-1)?.text).toBe("Hi again");
		expect(scheduler.pendingHandles).toBe(0);
	});

	it("finish(finalText) reveals the server's answer immediately and cancels the frame", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("Provisional delta text that streams in");
		pump(scheduler, 2);

		reveal.finish("The authoritative answer.");

		expect(emissions.at(-1)?.text).toBe("The authoritative answer.");
		expect(reveal.revealedLength).toBe("The authoritative answer.".length);
		expect(reveal.pendingLength).toBe(0);
		expect(scheduler.pendingHandles).toBe(0);
	});

	it("finish() with no argument reveals everything pushed so far", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("Hello, ");
		reveal.push("world");
		pump(scheduler, 2);

		reveal.finish();

		expect(emissions.at(-1)?.text).toBe("Hello, world");
		expect(scheduler.pendingHandles).toBe(0);
	});

	it("ignores push() after finish()", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("Hello");
		reveal.finish();
		const emitted = emissions.length;

		reveal.push(" ignored");
		pump(scheduler, 10);

		expect(emissions).toHaveLength(emitted);
		expect(emissions.at(-1)?.text).toBe("Hello");
		expect(reveal.pendingLength).toBe(0);
	});

	// The regression ADR-000985 names: the old ChatWindow never cancelled its
	// typewriter on destroy, so the timer chain kept writing after unmount.
	it("stop() drops the backlog, cancels the frame, and emits nothing more", () => {
		const { scheduler, emissions, reveal } = setup();

		reveal.push("q".repeat(1000));
		pump(scheduler, 3);
		const emitted = emissions.length;
		const outstanding = scheduler.requestCount;
		expect(scheduler.pendingHandles).toBe(1);

		reveal.stop();

		expect(scheduler.cancelled).toContain(outstanding);
		expect(scheduler.pendingHandles).toBe(0);

		pump(scheduler, 100);
		reveal.push("late delta");
		pump(scheduler, 100);

		expect(emissions).toHaveLength(emitted);
	});

	it("keeps stop() and finish() idempotent and safe before the first push", () => {
		const stopped = setup();
		expect(() => {
			stopped.reveal.stop();
			stopped.reveal.stop();
			stopped.reveal.finish();
		}).not.toThrow();
		expect(stopped.emissions).toEqual([]);

		// A drained finish emits nothing — there is nothing left to reveal.
		const finished = setup();
		finished.reveal.finish();
		finished.reveal.finish();
		expect(finished.emissions).toEqual([]);

		// The first finish wins; a second one cannot rewrite the answer.
		const twice = setup();
		twice.reveal.push("draft");
		twice.reveal.finish("first answer");
		twice.reveal.finish("second answer");
		expect(twice.emissions.at(-1)?.text).toBe("first answer");
	});

	it("reveals on push without ever scheduling a frame in immediate mode", () => {
		const { scheduler, emissions, reveal } = setup({ immediate: true });

		reveal.push("Hello");
		reveal.push(" reduced motion");

		expect(emissions.map((e) => e.text)).toEqual([
			"Hello",
			"Hello reduced motion",
		]);
		expect(scheduler.requestCount).toBe(0);
		expect(scheduler.pendingHandles).toBe(0);
	});

	it("never emits a lone surrogate", () => {
		const { scheduler, emissions, reveal } = setup();
		const text = "👍あ😀漢字🎉おわり";

		reveal.push(text);
		pump(scheduler, 100);

		expect(emissions.length).toBeGreaterThan(1);
		for (const e of emissions) {
			const last = e.text.charCodeAt(e.text.length - 1);
			expect(last >= 0xd800 && last <= 0xdbff).toBe(false);
		}
		expect(emissions.at(-1)?.text).toBe(text);
	});
});
