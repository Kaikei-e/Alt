import type { ReadingSurface } from "#lib/gen/alt/reading/v1/reading_pb.js";
import { createDwellTracker } from "./dwell-tracker.js";
import type { SessionEndedParams } from "./reading-events.js";

export interface ReadingSessionDwell {
	stop(): number;
}

export interface ReadingSessionQueue {
	enqueueReachedEnd(articleId: string, surface: ReadingSurface): void;
	enqueueSessionEnded(params: SessionEndedParams): void;
}

export interface ReadingSessionOptions {
	articleId: string;
	surface: ReadingSurface;
	bodyLengthChars: number;
	queue: ReadingSessionQueue;
	dwell?: ReadingSessionDwell;
}

export interface ReadingSession {
	readonly isEnded: boolean;
	onScroll(ratio: number): void;
	markReachedEnd(): void;
	end(): void;
}

export function createReadingSession(
	options: ReadingSessionOptions,
): ReadingSession {
	if (!options.articleId || options.articleId.trim().length === 0) {
		throw new Error("articleId is required to start a reading session");
	}

	const { articleId, surface, queue } = options;
	const bodyLengthChars =
		Number.isFinite(options.bodyLengthChars) && options.bodyLengthChars > 0
			? Math.trunc(options.bodyLengthChars)
			: 0;
	const dwell = options.dwell ?? createDwellTracker();

	let maxScrollRatio = 0;
	let reachedEnd = false;
	let ended = false;

	return {
		get isEnded() {
			return ended;
		},
		onScroll(ratio: number) {
			if (ended || Number.isNaN(ratio)) {
				return;
			}
			const clamped = Math.min(1, Math.max(0, ratio));
			if (clamped > maxScrollRatio) {
				maxScrollRatio = clamped;
			}
		},
		markReachedEnd() {
			if (ended || reachedEnd) {
				return;
			}
			reachedEnd = true;
			queue.enqueueReachedEnd(articleId, surface);
		},
		end() {
			if (ended) {
				return;
			}
			ended = true;
			const activeDwellMs = dwell.stop();
			queue.enqueueSessionEnded({
				articleId,
				surface,
				activeDwellMs,
				maxScrollRatio,
				bodyLengthChars,
				reachedEnd,
			});
		},
	};
}
