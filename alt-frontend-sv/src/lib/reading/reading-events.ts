import { create } from "@bufbuild/protobuf";
import {
	type Timestamp,
	timestampFromDate,
	timestampFromMs,
} from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, type Transport } from "@connectrpc/connect";
import {
	createKeepaliveTransport,
	recordReadingEvents,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import {
	ReachedEndSchema,
	type ReadingEvent,
	ReadingEventSchema,
	ReadingSurface,
	SessionEndedSchema,
	SurfaceViewedSchema,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import { uuidv7 } from "#lib/utils/uuidv7.js";

export { type ReadingEvent, ReadingSurface };

export const MAX_FLUSH_BATCH_SIZE = 50;
export const MAX_KEEPALIVE_BATCH_SIZE = 20;
export const MAX_PENDING_EVENTS = 200;

const PERMANENT_ERROR_CODES = new Set<Code>([
	Code.InvalidArgument,
	Code.PermissionDenied,
	Code.Unauthenticated,
	Code.FailedPrecondition,
	Code.Unimplemented,
	Code.NotFound,
]);

export interface SessionEndedParams {
	articleId: string;
	surface: ReadingSurface;
	activeDwellMs: number | bigint;
	maxScrollRatio: number;
	bodyLengthChars: number;
	reachedEnd: boolean;
}

export interface FlushOptions {
	keepalive?: boolean;
}

export interface ReadingEventQueueOptions {
	clock?: () => Date | number;
	generateId?: () => string;
	getTransport?: (keepalive: boolean) => Transport;
	send?: (
		events: ReadingEvent[],
		options: { keepalive: boolean },
	) => Promise<unknown>;
}

export interface ReadingEventQueue {
	readonly size: number;
	enqueueSurfaceViewed(surface: ReadingSurface): void;
	enqueueReachedEnd(articleId: string, surface: ReadingSurface): void;
	enqueueSessionEnded(params: SessionEndedParams): void;
	flush(options?: FlushOptions): Promise<void>;
}

function toTimestamp(value: Date | number): Timestamp {
	return value instanceof Date
		? timestampFromDate(value)
		: timestampFromMs(value);
}

function normalizeDwellMs(value: number | bigint): bigint {
	if (typeof value === "bigint") {
		return value < 0n ? 0n : value;
	}
	if (!Number.isFinite(value) || value <= 0) {
		return 0n;
	}
	return BigInt(Math.round(value));
}

function clampScrollRatio(value: number): number {
	if (!Number.isFinite(value)) {
		return 0;
	}
	return Math.min(1, Math.max(0, value));
}

function normalizeBodyLengthChars(value: number): number {
	if (!Number.isFinite(value) || value <= 0) {
		return 0;
	}
	return Math.trunc(value);
}

export function createReadingEventQueue(
	options: ReadingEventQueueOptions = {},
): ReadingEventQueue {
	const clock = options.clock ?? (() => Date.now());
	const generateId = options.generateId ?? uuidv7;
	const getTransport =
		options.getTransport ??
		((keepalive: boolean) =>
			keepalive ? createKeepaliveTransport() : createClientTransport());
	const send =
		options.send ??
		((events: ReadingEvent[], sendOptions: { keepalive: boolean }) =>
			recordReadingEvents(getTransport(sendOptions.keepalive), events));

	const pending: ReadingEvent[] = [];

	const enforceQueueCap = () => {
		if (pending.length > MAX_PENDING_EVENTS) {
			pending.splice(0, pending.length - MAX_PENDING_EVENTS);
		}
	};

	const enqueue = (event: ReadingEvent) => {
		pending.push(event);
		enforceQueueCap();
	};

	return {
		get size() {
			return pending.length;
		},
		enqueueSurfaceViewed(surface: ReadingSurface) {
			enqueue(
				create(ReadingEventSchema, {
					clientEventId: generateId(),
					occurredAt: toTimestamp(clock()),
					event: {
						case: "surfaceViewed",
						value: create(SurfaceViewedSchema, { surface }),
					},
				}),
			);
		},
		enqueueReachedEnd(articleId: string, surface: ReadingSurface) {
			enqueue(
				create(ReadingEventSchema, {
					clientEventId: generateId(),
					occurredAt: toTimestamp(clock()),
					event: {
						case: "reachedEnd",
						value: create(ReachedEndSchema, { articleId, surface }),
					},
				}),
			);
		},
		enqueueSessionEnded(params: SessionEndedParams) {
			enqueue(
				create(ReadingEventSchema, {
					clientEventId: generateId(),
					occurredAt: toTimestamp(clock()),
					event: {
						case: "sessionEnded",
						value: create(SessionEndedSchema, {
							articleId: params.articleId,
							surface: params.surface,
							activeDwellMs: normalizeDwellMs(params.activeDwellMs),
							maxScrollRatio: clampScrollRatio(params.maxScrollRatio),
							bodyLengthChars: normalizeBodyLengthChars(params.bodyLengthChars),
							reachedEnd: Boolean(params.reachedEnd),
						}),
					},
				}),
			);
		},
		async flush(flushOptions: FlushOptions = {}): Promise<void> {
			const keepalive = flushOptions.keepalive ?? false;
			const batchLimit = keepalive
				? MAX_KEEPALIVE_BATCH_SIZE
				: MAX_FLUSH_BATCH_SIZE;

			while (pending.length > 0) {
				const batch = pending.splice(0, batchLimit);
				try {
					await send(batch, { keepalive });
				} catch (err) {
					if (PERMANENT_ERROR_CODES.has(ConnectError.from(err).code)) {
						continue;
					}
					// Server deduplicates by client_event_id, so transient failures return to the head for retry.
					pending.unshift(...batch);
					enforceQueueCap();
					return;
				}
			}
		},
	};
}
