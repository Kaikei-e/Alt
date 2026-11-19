import type { MessageInitShape } from "@bufbuild/protobuf";
import { timestampDate, timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { Client, Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import {
	type GetReadTodayQueueResponse,
	QueueArticleOutcome,
	type QueueArticleResponse,
	type ReadingEvent,
	type ReadingEventSchema,
	type ReadingQueueItem,
	ReadingService,
	ReadingSurface,
	type RecordReadingEventsResponse,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import { uuidv7 } from "#lib/utils/uuidv7.js";

export { QueueArticleOutcome, type ReadingEvent, ReadingSurface };

export type ReadingClient = Client<typeof ReadingService>;

export type ReadingEventInput = MessageInitShape<typeof ReadingEventSchema>;

export interface RecordReadingEventsResult {
	accepted: number;
	duplicates: number;
	readArticleIds: string[];
}

export interface ReadTodayQueueItem {
	articleId: string;
	title: string;
	url: string;
	sourceName: string;
	imageUrl: string;
	queuedAt: Date | null;
	expiresAt: Date | null;
}

export interface QueueArticleParams {
	articleId: string;
	surface: ReadingSurface;
	clientEventId?: string;
	occurredAt?: Date;
}

export interface QueueArticleResult {
	outcome: QueueArticleOutcome;
	items: ReadTodayQueueItem[];
	cap: number;
}

export interface UnqueueArticleParams {
	articleId: string;
	clientEventId?: string;
	occurredAt?: Date;
}

export interface GetReadTodayQueueResult {
	items: ReadTodayQueueItem[];
	cap: number;
	readCountSince: number;
}

function mapQueueItem(item: ReadingQueueItem): ReadTodayQueueItem {
	return {
		articleId: item.articleId,
		title: item.title,
		url: item.url,
		sourceName: item.sourceName,
		imageUrl: item.imageUrl,
		queuedAt: item.queuedAt ? timestampDate(item.queuedAt) : null,
		expiresAt: item.expiresAt ? timestampDate(item.expiresAt) : null,
	};
}

export function isQueueSuccessOutcome(outcome: QueueArticleOutcome): boolean {
	return (
		outcome === QueueArticleOutcome.QUEUED ||
		outcome === QueueArticleOutcome.ALREADY_QUEUED
	);
}

export function createReadingClient(transport: Transport): ReadingClient {
	return createClient(ReadingService, transport);
}

export async function recordReadingEvents(
	transport: Transport,
	events: ReadingEventInput[],
): Promise<RecordReadingEventsResult> {
	const client = createReadingClient(transport);
	const response: RecordReadingEventsResponse =
		await client.recordReadingEvents({ events });
	return {
		accepted: response.accepted,
		duplicates: response.duplicates,
		readArticleIds: response.readArticleIds ?? [],
	};
}

export async function queueArticle(
	transport: Transport,
	params: QueueArticleParams,
): Promise<QueueArticleResult> {
	const client = createReadingClient(transport);
	const response: QueueArticleResponse = await client.queueArticle({
		articleId: params.articleId,
		clientEventId: params.clientEventId ?? uuidv7(),
		occurredAt: timestampFromDate(params.occurredAt ?? new Date()),
		surface: params.surface,
	});
	return {
		outcome: response.outcome,
		items: response.items.map(mapQueueItem),
		cap: response.cap,
	};
}

export async function unqueueArticle(
	transport: Transport,
	params: UnqueueArticleParams,
): Promise<void> {
	const client = createReadingClient(transport);
	await client.unqueueArticle({
		articleId: params.articleId,
		clientEventId: params.clientEventId ?? uuidv7(),
		occurredAt: timestampFromDate(params.occurredAt ?? new Date()),
	});
}

export async function getReadTodayQueue(
	transport: Transport,
	readSince: Date,
): Promise<GetReadTodayQueueResult> {
	const client = createReadingClient(transport);
	const response: GetReadTodayQueueResponse = await client.getReadTodayQueue({
		readSince: timestampFromDate(readSince),
	});
	return {
		items: response.items.map(mapQueueItem),
		cap: response.cap,
		readCountSince: response.readCountSince,
	};
}
