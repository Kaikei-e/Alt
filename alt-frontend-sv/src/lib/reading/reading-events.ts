import { create } from "@bufbuild/protobuf";
import {
	type Timestamp,
	timestampFromDate,
	timestampFromMs,
} from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, type Transport } from "@connectrpc/connect";
import { recordReadingEvents } from "#lib/connect/reading.js";
import {
	OpenedSchema,
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
export const MAX_RETRY_ATTEMPTS = 3;

const UUID_PATTERN =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUuid(value: string | null | undefined): value is string {
	return typeof value === "string" && UUID_PATTERN.test(value.trim());
}

const PERMANENT_ERROR_CODES = new Set<Code>([
	Code.InvalidArgument,
	Code.PermissionDenied,
	Code.Unauthenticated,
	Code.FailedPrecondition,
	Code.Unimplemented,
	Code.NotFound,
]);

const UNREACHABLE_ERROR_CODES = new Set<Code>([
	Code.Unavailable,
	Code.DeadlineExceeded,
	Code.Canceled,
	Code.Aborted,
]);

function isUnreachableError(err: unknown, connectErr: ConnectError): boolean {
	if (UNREACHABLE_ERROR_CODES.has(connectErr.code)) {
		return true;
	}
	if (connectErr.code === Code.Unknown) {
		return err instanceof TypeError || connectErr.cause instanceof TypeError;
	}
	return false;
}

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
	onReadArticleIds?: (articleIds: string[]) => void;
}

export interface ReadingEventQueue {
	readonly size: number;
	enqueueSurfaceViewed(surface: ReadingSurface): void;
	enqueueOpened(articleId: string, surface: ReadingSurface): void;
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
	let send: NonNullable<ReadingEventQueueOptions["send"]>;
	if (options.send) {
		send = options.send;
	} else if (options.getTransport) {
		const getTransport = options.getTransport;
		send = (events, sendOptions) =>
			recordReadingEvents(getTransport(sendOptions.keepalive), events);
	} else {
		throw new Error(
			"createReadingEventQueue requires either send or getTransport",
		);
	}

	interface RetryBatch {
		events: ReadingEvent[];
		attempts: number;
	}

	const retryBatches: RetryBatch[] = [];
	const pending: ReadingEvent[] = [];

	const totalSize = () => {
		let total = pending.length;
		for (const batch of retryBatches) {
			total += batch.events.length;
		}
		return total;
	};

	const warnDropped = (count: number, reason: string) => {
		if (count <= 0) {
			return;
		}
		console.warn(
			`[reading-events] Dropped ${count} reading event(s) (${reason})`,
		);
	};

	const enforceQueueCap = () => {
		let excess = totalSize() - MAX_PENDING_EVENTS;
		if (excess <= 0) {
			return;
		}
		let evicted = 0;
		while (excess > 0 && retryBatches.length > 0) {
			const oldest = retryBatches[0];
			if (!oldest) break;
			if (oldest.events.length <= excess) {
				evicted += oldest.events.length;
				excess -= oldest.events.length;
				retryBatches.shift();
			} else {
				oldest.events.splice(0, excess);
				evicted += excess;
				excess = 0;
			}
		}
		if (excess > 0 && pending.length > 0) {
			const dropFromPending = Math.min(excess, pending.length);
			pending.splice(0, dropFromPending);
			evicted += dropFromPending;
		}
		warnDropped(evicted, "queue capacity exceeded");
	};

	const enqueue = (event: ReadingEvent) => {
		if (
			event.event.case !== "surfaceViewed" &&
			!isUuid(event.event.value?.articleId)
		) {
			return;
		}
		pending.push(event);
		enforceQueueCap();
	};

	return {
		get size() {
			return totalSize();
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
		enqueueOpened(articleId: string, surface: ReadingSurface) {
			const normalizedId = articleId.trim();
			if (!isUuid(normalizedId)) {
				return;
			}
			enqueue(
				create(ReadingEventSchema, {
					clientEventId: generateId(),
					occurredAt: toTimestamp(clock()),
					event: {
						case: "opened",
						value: create(OpenedSchema, { articleId: normalizedId, surface }),
					},
				}),
			);
		},
		enqueueReachedEnd(articleId: string, surface: ReadingSurface) {
			const normalizedId = articleId.trim();
			if (!isUuid(normalizedId)) {
				return;
			}
			enqueue(
				create(ReadingEventSchema, {
					clientEventId: generateId(),
					occurredAt: toTimestamp(clock()),
					event: {
						case: "reachedEnd",
						value: create(ReachedEndSchema, {
							articleId: normalizedId,
							surface,
						}),
					},
				}),
			);
		},
		enqueueSessionEnded(params: SessionEndedParams) {
			const normalizedId = params.articleId.trim();
			if (!isUuid(normalizedId)) {
				return;
			}
			enqueue(
				create(ReadingEventSchema, {
					clientEventId: generateId(),
					occurredAt: toTimestamp(clock()),
					event: {
						case: "sessionEnded",
						value: create(SessionEndedSchema, {
							articleId: normalizedId,
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
			const attemptedThisFlush = new Set<RetryBatch>();

			const takeNextBatch = (): RetryBatch | null => {
				const retryIndex = retryBatches.findIndex(
					(batch) => !attemptedThisFlush.has(batch),
				);
				if (retryIndex !== -1) {
					const retryBatch = retryBatches[retryIndex];
					if (retryBatch) {
						if (retryBatch.events.length <= batchLimit) {
							retryBatches.splice(retryIndex, 1);
							return retryBatch;
						}
						return {
							events: retryBatch.events.splice(0, batchLimit),
							attempts: retryBatch.attempts,
						};
					}
				}
				if (pending.length > 0) {
					return {
						events: pending.splice(0, batchLimit),
						attempts: 0,
					};
				}
				return null;
			};

			while (true) {
				const batch = takeNextBatch();
				if (!batch) {
					break;
				}
				try {
					const response = await send(batch.events, { keepalive });
					if (
						response &&
						typeof response === "object" &&
						"readArticleIds" in response &&
						Array.isArray(response.readArticleIds) &&
						response.readArticleIds.length > 0
					) {
						const ids = response.readArticleIds.filter(
							(id): id is string =>
								typeof id === "string" && id.trim().length > 0,
						);
						if (ids.length > 0) {
							options.onReadArticleIds?.(ids);
						}
					}
				} catch (err) {
					const connectErr = ConnectError.from(err);
					if (PERMANENT_ERROR_CODES.has(connectErr.code)) {
						continue;
					}
					const unreachable = isUnreachableError(err, connectErr);
					const nextAttempts = unreachable
						? batch.attempts
						: batch.attempts + 1;
					if (!unreachable && nextAttempts >= MAX_RETRY_ATTEMPTS) {
						warnDropped(batch.events.length, "retry limit reached");
						continue;
					}
					// Server deduplicates by client_event_id; retain for a later flush without blocking batches behind it.
					const retryEntry: RetryBatch = {
						events: batch.events,
						attempts: nextAttempts,
					};
					attemptedThisFlush.add(retryEntry);
					retryBatches.push(retryEntry);
					enforceQueueCap();
				}
			}
		},
	};
}
