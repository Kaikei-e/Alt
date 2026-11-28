import type { MessageInitShape } from "@bufbuild/protobuf";
import { timestampDate, timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { Client, Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import {
	AnnotationStance,
	type ArticleAnnotation,
	type CurateThemeItemResponse,
	type GetReadTodayQueueResponse,
	type GetThemeResponse,
	type ListArticleAnnotationsResponse,
	type ListThemeCurationsResponse,
	type ListThemesResponse,
	QueueArticleOutcome,
	type QueueArticleResponse,
	type ReadingEvent,
	type ReadingEventSchema,
	type ReadingQueueItem,
	ReadingService,
	ReadingSurface,
	type RecordReadingEventsResponse,
	type SaveArticleAnnotationResponse,
	type ThemeCuration,
	ThemeCurationKind,
	type ThemeSentence,
	type ThemeSource,
	type ThemeSummary,
	type ThemeTimelineEntry,
	type ThemeVersionRef,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import { uuidv7 } from "#lib/utils/uuidv7.js";

export {
	AnnotationStance,
	QueueArticleOutcome,
	type ReadingEvent,
	ReadingSurface,
	ThemeCurationKind,
};

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

export interface ThemeSummaryItem {
	themeId: string;
	title: string;
	latestVersion: number;
	articleCount: number;
	updatedAt: Date | null;
	lastOpenedAt: Date | null;
	lastOpenedVersion: number;
}

export interface ThemeSentenceItem {
	text: string;
	sourceArticleIds: string[];
}

export interface ThemeTimelineItem {
	date: string;
	text: string;
	sourceArticleIds: string[];
}

export interface ThemeSourceItem {
	articleId: string;
	title: string;
	url: string;
}

export interface ThemeVersionItem {
	version: number;
	createdAt: Date | null;
	articleCount: number;
}

export interface ThemePageData {
	themeId: string;
	title: string;
	version: number;
	latestVersion: number;
	createdAt: Date | null;
	keyPoints: ThemeSentenceItem[];
	timeline: ThemeTimelineItem[];
	whatChanged: ThemeSentenceItem[];
	openQuestions: ThemeSentenceItem[];
	sources: ThemeSourceItem[];
	versions: ThemeVersionItem[];
}

export interface RecordThemeOpenedOptions {
	clientEventId?: string;
	openedAt?: Date;
}

export interface ArticleAnnotationItem {
	annotationId: string;
	articleId: string;
	stance: AnnotationStance;
	excerpt: string;
	note: string;
	createdAt: Date | null;
}

export interface SaveArticleAnnotationParams {
	articleId: string;
	stance: AnnotationStance;
	excerpt?: string;
	note?: string;
	clientEventId?: string;
	createdAt?: Date;
}

export interface DeleteArticleAnnotationOptions {
	clientEventId?: string;
	deletedAt?: Date;
}

export interface ThemeCurationItem {
	curationId: string;
	themeId: string;
	kind: ThemeCurationKind;
	targetHash: string;
	targetText: string;
	userNote: string;
	augurConversationId: string;
	createdAt: Date | null;
}

export interface CurateThemeItemParams {
	themeId: string;
	kind: ThemeCurationKind;
	targetHash?: string;
	targetText?: string;
	userNote?: string;
	augurConversationId?: string;
	clientEventId?: string;
	createdAt?: Date;
}

export interface RemoveThemeCurationOptions {
	clientEventId?: string;
	removedAt?: Date;
}

function mapArticleAnnotation(item: ArticleAnnotation): ArticleAnnotationItem {
	return {
		annotationId: item.annotationId,
		articleId: item.articleId,
		stance: item.stance,
		excerpt: item.excerpt,
		note: item.note,
		createdAt: item.createdAt ? timestampDate(item.createdAt) : null,
	};
}

function mapThemeCuration(item: ThemeCuration): ThemeCurationItem {
	return {
		curationId: item.curationId,
		themeId: item.themeId,
		kind: item.kind,
		targetHash: item.targetHash,
		targetText: item.targetText,
		userNote: item.userNote,
		augurConversationId: item.augurConversationId,
		createdAt: item.createdAt ? timestampDate(item.createdAt) : null,
	};
}

function mapThemeSummary(item: ThemeSummary): ThemeSummaryItem {
	return {
		themeId: item.themeId,
		title: item.title,
		latestVersion: item.latestVersion,
		articleCount: item.articleCount,
		updatedAt: item.updatedAt ? timestampDate(item.updatedAt) : null,
		lastOpenedAt: item.lastOpenedAt ? timestampDate(item.lastOpenedAt) : null,
		lastOpenedVersion: item.lastOpenedVersion,
	};
}

function mapThemeSentence(sentence: ThemeSentence): ThemeSentenceItem {
	return {
		text: sentence.text,
		sourceArticleIds: [...sentence.sourceArticleIds],
	};
}

function mapThemeTimeline(entry: ThemeTimelineEntry): ThemeTimelineItem {
	return {
		date: entry.date,
		text: entry.text,
		sourceArticleIds: [...entry.sourceArticleIds],
	};
}

function mapThemeSource(source: ThemeSource): ThemeSourceItem {
	return {
		articleId: source.articleId,
		title: source.title,
		url: source.url,
	};
}

function mapThemeVersion(ref: ThemeVersionRef): ThemeVersionItem {
	return {
		version: ref.version,
		createdAt: ref.createdAt ? timestampDate(ref.createdAt) : null,
		articleCount: ref.articleCount,
	};
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

export async function listThemes(
	transport: Transport,
): Promise<ThemeSummaryItem[]> {
	const client = createReadingClient(transport);
	const response: ListThemesResponse = await client.listThemes({});
	return response.themes.map(mapThemeSummary);
}

export async function getTheme(
	transport: Transport,
	themeId: string,
	version?: number,
): Promise<ThemePageData> {
	const client = createReadingClient(transport);
	const response: GetThemeResponse = await client.getTheme(
		version !== undefined ? { themeId, version } : { themeId },
	);
	return {
		themeId: response.themeId,
		title: response.title,
		version: response.version,
		latestVersion: response.latestVersion,
		createdAt: response.createdAt ? timestampDate(response.createdAt) : null,
		keyPoints: response.keyPoints.map(mapThemeSentence),
		timeline: response.timeline.map(mapThemeTimeline),
		whatChanged: response.whatChanged.map(mapThemeSentence),
		openQuestions: response.openQuestions.map(mapThemeSentence),
		sources: response.sources.map(mapThemeSource),
		versions: response.versions.map(mapThemeVersion),
	};
}

export async function recordThemeOpened(
	transport: Transport,
	themeId: string,
	version: number,
	options?: RecordThemeOpenedOptions,
): Promise<void> {
	try {
		const client = createReadingClient(transport);
		await client.recordThemeOpened({
			themeId,
			version,
			clientEventId: options?.clientEventId ?? uuidv7(),
			openedAt: timestampFromDate(options?.openedAt ?? new Date()),
		});
	} catch {
		// Telemetry must never block reading; omit identifiers from logs.
		console.warn("Failed to record theme opened");
	}
}

export async function saveArticleAnnotation(
	transport: Transport,
	params: SaveArticleAnnotationParams,
): Promise<ArticleAnnotationItem> {
	const client = createReadingClient(transport);
	const response: SaveArticleAnnotationResponse =
		await client.saveArticleAnnotation({
			articleId: params.articleId,
			stance: params.stance,
			excerpt: params.excerpt ?? "",
			note: params.note ?? "",
			clientEventId: params.clientEventId ?? uuidv7(),
			createdAt: timestampFromDate(params.createdAt ?? new Date()),
		});
	if (!response.annotation) {
		throw new Error("SaveArticleAnnotation returned empty annotation");
	}
	return mapArticleAnnotation(response.annotation);
}

export async function deleteArticleAnnotation(
	transport: Transport,
	annotationId: string,
	options?: DeleteArticleAnnotationOptions,
): Promise<void> {
	const client = createReadingClient(transport);
	await client.deleteArticleAnnotation({
		annotationId,
		clientEventId: options?.clientEventId ?? uuidv7(),
		deletedAt: timestampFromDate(options?.deletedAt ?? new Date()),
	});
}

export async function listArticleAnnotations(
	transport: Transport,
	articleIds: string[] = [],
	limit = 0,
): Promise<ArticleAnnotationItem[]> {
	const client = createReadingClient(transport);
	const response: ListArticleAnnotationsResponse =
		await client.listArticleAnnotations({
			articleIds,
			limit,
		});
	return response.annotations.map(mapArticleAnnotation);
}

export async function curateThemeItem(
	transport: Transport,
	params: CurateThemeItemParams,
): Promise<ThemeCurationItem> {
	const client = createReadingClient(transport);
	const response: CurateThemeItemResponse = await client.curateThemeItem({
		themeId: params.themeId,
		kind: params.kind,
		targetHash: params.targetHash ?? "",
		targetText: params.targetText ?? "",
		userNote: params.userNote ?? "",
		augurConversationId: params.augurConversationId ?? "",
		clientEventId: params.clientEventId ?? uuidv7(),
		createdAt: timestampFromDate(params.createdAt ?? new Date()),
	});
	if (!response.curation) {
		throw new Error("CurateThemeItem returned empty curation");
	}
	return mapThemeCuration(response.curation);
}

export async function removeThemeCuration(
	transport: Transport,
	curationId: string,
	options?: RemoveThemeCurationOptions,
): Promise<void> {
	const client = createReadingClient(transport);
	await client.removeThemeCuration({
		curationId,
		clientEventId: options?.clientEventId ?? uuidv7(),
		removedAt: timestampFromDate(options?.removedAt ?? new Date()),
	});
}

export async function listThemeCurations(
	transport: Transport,
	themeId: string,
	limit = 0,
): Promise<ThemeCurationItem[]> {
	const client = createReadingClient(transport);
	const response: ListThemeCurationsResponse = await client.listThemeCurations({
		themeId,
		limit,
	});
	return response.curations.map(mapThemeCuration);
}
