/**
 * Typewriter reveal for streamed answers — the one shared engine behind the
 * ASK Augur chat (AugurChat.svelte) and the Knowledge Home ask pane
 * (useAugurPane.svelte.ts), per ADR-000985: the effect exists in one place for
 * everyone or not at all.
 *
 * Unlike a fixed-rate typewriter, this is a proportional drain: each animation
 * frame reveals a slice of the backlog sized so the backlog decays e-fold per
 * `horizonMs`. A stream that runs ahead makes the reveal speed up instead of
 * falling behind, so completion never has to slam in a paragraph the crawl
 * could not keep up with — `finish()` is still a hard flush, but by then the
 * backlog is bounded to roughly one horizon.
 *
 * Note: `simulateTypewriterEffect` in ./streamingRenderer.ts is the superseded
 * fixed-rate engine, retained only because the swipe cards (SwipeFeedCard /
 * VisualPreviewCard) still render through it. New streaming surfaces use this
 * module.
 */

export interface RevealScheduler {
	/** Current time in ms. Monotonic. */
	now(): number;
	/** Schedule one callback for the next frame; returns a cancel handle. */
	request(cb: (t: number) => void): number;
	/** Cancel a handle returned by request(). */
	cancel(handle: number): void;
}

export interface TypewriterRevealOptions {
	/** Called with the full revealed prefix each time it grows. */
	onReveal: (text: string) => void;
	/** Exponential time constant. Backlog decays e-fold per horizon. Default 220. */
	horizonMs?: number;
	/** Floor so a 2-char backlog still moves. Default 24. */
	minCharsPerSecond?: number;
	/** Ceiling so one frame never dumps a paragraph. Default 24. */
	maxCharsPerFrame?: number;
	/** Longest dt honoured — a backgrounded tab must not dump on resume. Default 100. */
	maxFrameMs?: number;
	/** prefers-reduced-motion: reveal on push, never schedule a frame. Default false. */
	immediate?: boolean;
	/** Defaults to requestAnimationFrame + performance.now. */
	scheduler?: RevealScheduler;
}

export interface TypewriterReveal {
	/** A stream delta arrived. No-op after finish() or stop(). */
	push(delta: string): void;
	/**
	 * Stream done: reveal everything, now, and stop. When `finalText` is given
	 * it REPLACES the accumulated deltas — the server's answer is authoritative.
	 * Idempotent.
	 */
	finish(finalText?: string): void;
	/** Destroy/abort: drop the backlog with no emit and stop. Idempotent. */
	stop(): void;
	readonly revealedLength: number;
	readonly pendingLength: number;
}

const DEFAULT_HORIZON_MS = 220;
const DEFAULT_MIN_CHARS_PER_SECOND = 24;
const DEFAULT_MAX_CHARS_PER_FRAME = 24;
const DEFAULT_MAX_FRAME_MS = 100;

// Closures, not bindings, so merely importing this module is safe where
// requestAnimationFrame does not exist (SSR, node tests). The scheduler is
// only ever invoked from a running stream, which is client-only.
const rafScheduler: RevealScheduler = {
	now: () => performance.now(),
	request: (cb) => requestAnimationFrame(cb),
	cancel: (handle) => cancelAnimationFrame(handle),
};

/**
 * A reveal boundary must never land between the halves of a surrogate pair:
 * a split emoji renders as U+FFFD. If the character before `index` is a high
 * surrogate, hold that character back for the next frame.
 */
function avoidLoneSurrogate(text: string, index: number): number {
	const preceding = text.charCodeAt(index - 1);
	if (preceding >= 0xd800 && preceding <= 0xdbff) {
		return index - 1;
	}
	return index;
}

export function createTypewriterReveal(
	options: TypewriterRevealOptions,
): TypewriterReveal {
	const {
		onReveal,
		horizonMs = DEFAULT_HORIZON_MS,
		minCharsPerSecond = DEFAULT_MIN_CHARS_PER_SECOND,
		maxCharsPerFrame = DEFAULT_MAX_CHARS_PER_FRAME,
		maxFrameMs = DEFAULT_MAX_FRAME_MS,
		immediate = false,
		scheduler = rafScheduler,
	} = options;

	let target = "";
	let revealed = 0;
	let credit = 0;
	let handle: number | null = null;
	let lastT = 0;
	let done = false;

	function cancelFrame(): void {
		if (handle !== null) {
			scheduler.cancel(handle);
			handle = null;
		}
	}

	function frame(t: number): void {
		handle = null;
		const dt = Math.min(t - lastT, maxFrameMs);
		lastT = t;

		const backlog = target.length - revealed;
		if (backlog <= 0) {
			// Drained: go idle. Banked credit dies with the burst so the next
			// push starts from a standstill instead of popping a frame's worth.
			credit = 0;
			return;
		}

		const cps = Math.max((backlog * 1000) / horizonMs, minCharsPerSecond);
		credit += (cps * dt) / 1000;
		const n = Math.min(Math.floor(credit), maxCharsPerFrame, backlog);
		const next = avoidLoneSurrogate(target, revealed + n);
		// Deduct only what was actually revealed: a boundary held back for a
		// surrogate pair keeps its credit, or the loop would stall one short of
		// the pair forever.
		const advanced = next - revealed;
		credit = Math.min(credit - advanced, maxCharsPerFrame);

		if (advanced > 0) {
			revealed = next;
			onReveal(target.slice(0, revealed));
		}

		if (revealed < target.length) {
			handle = scheduler.request(frame);
		} else {
			credit = 0;
		}
	}

	function push(delta: string): void {
		if (done || delta === "") return;
		target += delta;
		if (immediate) {
			revealed = target.length;
			onReveal(target);
			return;
		}
		if (handle === null) {
			lastT = scheduler.now();
			handle = scheduler.request(frame);
		}
	}

	function finish(finalText?: string): void {
		if (done) return;
		done = true;
		cancelFrame();
		const replaced = finalText !== undefined && finalText !== target;
		if (finalText !== undefined) {
			target = finalText;
		}
		// Emits nothing when already drained — completion of a fully revealed
		// answer is not a new frame of text.
		if (revealed !== target.length || replaced) {
			revealed = target.length;
			onReveal(target);
		}
	}

	function stop(): void {
		if (done) return;
		done = true;
		cancelFrame();
	}

	return {
		push,
		finish,
		stop,
		get revealedLength() {
			return revealed;
		},
		get pendingLength() {
			return target.length - revealed;
		},
	};
}
