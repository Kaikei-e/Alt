/**
 * KnowledgeHomeService client for Connect-RPC
 *
 * Provides type-safe methods to call KnowledgeHomeService endpoints.
 * Authentication is handled by the transport layer.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	KnowledgeHomeService,
	type GetKnowledgeHomeResponse,
	type KnowledgeHomeItem as ProtoKnowledgeHomeItem,
	type TodayDigest as ProtoTodayDigest,
	type WhyReason as ProtoWhyReason,
	type RecallCandidate as ProtoRecallCandidate,
	type Lens as ProtoLens,
	type SupersedeInfo as ProtoSupersedeInfo,
} from "$lib/gen/alt/knowledge_home/v1/knowledge_home_pb";

/** Type-safe KnowledgeHomeService client */
type KnowledgeHomeClient = Client<typeof KnowledgeHomeService>;

/** Why reason explaining why an item appeared */
export interface WhyReasonData {
	code: string;
	refId?: string;
	tag?: string;
}

/** Digest freshness indicator */
export type DigestFreshness = "fresh" | "stale" | "unknown";

/** Today's digest summary */
export interface TodayDigestData {
	date: string;
	newArticles: number;
	summarizedArticles: number;
	unsummarizedArticles: number;
	topTags: string[];
	weeklyRecapAvailable: boolean;
	eveningPulseAvailable: boolean;
	needToKnowCount: number;
	digestFreshness: DigestFreshness;
	lastProjectedAt: string | null;
}

/** Service quality level returned by Knowledge Home API */
export type ServiceQuality = "full" | "degraded" | "fallback";

/** Summary processing state */
export type SummaryState = "missing" | "pending" | "ready";

/** A single Knowledge Home item */
export interface KnowledgeHomeItemData {
	itemKey: string;
	itemType: string;
	articleId?: string;
	recapId?: string;
	title: string;
	publishedAt: string;
	summaryExcerpt?: string;
	summaryState: SummaryState;
	tags: string[];
	why: WhyReasonData[];
	score: number;
	supersedeInfo?: SupersedeInfoData;
	link?: string;
}

/** Supersede info for version changes */
export interface SupersedeInfoData {
	state: string;
	supersededAt: string;
	previousSummaryExcerpt?: string;
	previousTags: string[];
}

/** A recall candidate */
export interface RecallCandidateData {
	itemKey: string;
	recallScore: number;
	reasons: RecallReasonData[];
	firstEligibleAt: string;
	nextSuggestAt: string;
	item?: KnowledgeHomeItemData;
}

/** Recall reason */
export interface RecallReasonData {
	type: string;
	description: string;
	sourceItemKey?: string;
}

/** A saved lens viewpoint */
export interface LensData {
	lensId: string;
	name: string;
	description: string;
	createdAt: string;
	updatedAt: string;
	currentVersion?: LensVersionData;
}

/** Lens version configuration */
export interface LensVersionData {
	versionId: string;
	queryText: string;
	tagIds: string[];
	feedIds: string[];
	timeWindow: string;
	includeRecap: boolean;
	includePulse: boolean;
	sortMode: string;
}

export interface ListLensesResult {
	lenses: LensData[];
	activeLensId: string | null;
}

/** Stream home update event */
export interface StreamHomeUpdate {
	eventType: string;
	item?: KnowledgeHomeItemData;
	digestChange?: TodayDigestData;
	recallChange?: RecallCandidateData;
	occurredAt: string;
	reconnectAfterMs?: number;
}

/** Feature flag status */
export interface FeatureFlagData {
	name: string;
	enabled: boolean;
}

/** Result from getKnowledgeHome */
export interface KnowledgeHomeResult {
	items: KnowledgeHomeItemData[];
	digest: TodayDigestData | null;
	nextCursor: string;
	hasMore: boolean;
	degraded: boolean;
	generatedAt: string;
	featureFlags: FeatureFlagData[];
	recallCandidates: RecallCandidateData[];
	serviceQuality: ServiceQuality;
}

/**
 * Creates a KnowledgeHomeService client with the given transport.
 */
export function createKnowledgeHomeClient(
	transport: Transport,
): KnowledgeHomeClient {
	return createClient(KnowledgeHomeService, transport);
}

function convertWhyReason(proto: ProtoWhyReason): WhyReasonData {
	return {
		code: proto.code,
		refId: proto.refId || undefined,
		tag: proto.tag || undefined,
	};
}

function convertDigest(proto: ProtoTodayDigest): TodayDigestData {
	return {
		date: proto.date,
		newArticles: proto.newArticles,
		summarizedArticles: proto.summarizedArticles,
		unsummarizedArticles: proto.unsummarizedArticles,
		topTags: [...proto.topTags],
		weeklyRecapAvailable: proto.weeklyRecapAvailable,
		eveningPulseAvailable: proto.eveningPulseAvailable,
		needToKnowCount: proto.needToKnowCount,
		digestFreshness: (proto.digestFreshness || "unknown") as DigestFreshness,
		lastProjectedAt: proto.lastProjectedAt || null,
	};
}

function normalizeServiceQuality(
	serviceQuality: string | undefined,
	degraded: boolean,
): ServiceQuality {
	if (serviceQuality === "degraded" || serviceQuality === "fallback") {
		return serviceQuality;
	}
	return degraded ? "degraded" : "full";
}

function convertItem(proto: ProtoKnowledgeHomeItem): KnowledgeHomeItemData {
	return {
		itemKey: proto.itemKey,
		itemType: proto.itemType,
		articleId: proto.articleId || undefined,
		recapId: proto.recapId || undefined,
		title: proto.title,
		publishedAt: proto.publishedAt,
		summaryExcerpt: proto.summaryExcerpt || undefined,
		summaryState: (proto.summaryState || "missing") as SummaryState,
		tags: [...proto.tags],
		why: proto.why.map(convertWhyReason),
		score: proto.score,
		supersedeInfo: proto.supersedeInfo ? {
			state: proto.supersedeInfo.state,
			supersededAt: proto.supersedeInfo.supersededAt,
			previousSummaryExcerpt: proto.supersedeInfo.previousSummaryExcerpt || undefined,
			previousTags: [...(proto.supersedeInfo.previousTags || [])],
		} : undefined,
		link: proto.link || undefined,
	};
}

/**
 * Fetches the Knowledge Home feed.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Pagination cursor (optional)
 * @param limit - Max items to return (default 20)
 * @returns Knowledge Home items, digest, and pagination info
 */
export async function getKnowledgeHome(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
	lensId?: string,
): Promise<KnowledgeHomeResult> {
	const client = createKnowledgeHomeClient(transport);
	const response = (await client.getKnowledgeHome({
		cursor,
		limit,
		lensId,
	})) as GetKnowledgeHomeResponse;

	return {
		items: response.items.map(convertItem),
		digest: response.todayDigest ? convertDigest(response.todayDigest) : null,
		nextCursor: response.nextCursor,
		hasMore: response.hasMore,
		degraded: response.degradedMode,
		generatedAt: response.generatedAt,
		featureFlags: (response.featureFlags ?? []).map((f) => ({
			name: f.name,
			enabled: f.enabled,
		})),
		recallCandidates: (response.recallCandidates ?? []).map(convertRecallCandidate),
		serviceQuality: normalizeServiceQuality(
			response.serviceQuality,
			response.degradedMode,
		),
	};
}

/**
 * Records which items were visible on screen (batch impression tracking).
 *
 * @param transport - The Connect transport to use
 * @param itemKeys - Item keys that were visible
 * @param sessionId - Exposure session ID for deduplication
 */
export async function trackHomeItemsSeen(
	transport: Transport,
	itemKeys: string[],
	sessionId: string,
): Promise<void> {
	const client = createKnowledgeHomeClient(transport);
	await client.trackHomeItemsSeen({
		itemKeys,
		exposureSessionId: sessionId,
	});
}

/**
 * Records a user action on a home item.
 *
 * @param transport - The Connect transport to use
 * @param actionType - Action type (open, dismiss, ask, listen)
 * @param itemKey - The item key being acted upon
 * @param metadataJson - Optional metadata as JSON string
 */
export async function trackHomeAction(
	transport: Transport,
	actionType: string,
	itemKey: string,
	metadataJson?: string,
): Promise<void> {
	const client = createKnowledgeHomeClient(transport);
	await client.trackHomeAction({
		actionType,
		itemKey,
		metadataJson,
	});
}

function convertRecallCandidate(proto: ProtoRecallCandidate): RecallCandidateData {
	return {
		itemKey: proto.itemKey,
		recallScore: proto.recallScore,
		reasons: proto.reasons.map(r => ({
			type: r.type,
			description: r.description,
			sourceItemKey: r.sourceItemKey || undefined,
		})),
		firstEligibleAt: proto.firstEligibleAt,
		nextSuggestAt: proto.nextSuggestAt,
		item: proto.item ? convertItem(proto.item) : undefined,
	};
}

function convertLens(proto: ProtoLens): LensData {
	return {
		lensId: proto.lensId,
		name: proto.name,
		description: proto.description,
		createdAt: proto.createdAt,
		updatedAt: proto.updatedAt,
		currentVersion: proto.currentVersion ? {
			versionId: proto.currentVersion.versionId,
			queryText: proto.currentVersion.queryText,
			tagIds: [...proto.currentVersion.tagIds],
			feedIds: [...proto.currentVersion.feedIds],
			timeWindow: proto.currentVersion.timeWindow,
			includeRecap: proto.currentVersion.includeRecap,
			includePulse: proto.currentVersion.includePulse,
			sortMode: proto.currentVersion.sortMode,
		} : undefined,
	};
}

export async function getRecallRailCandidates(
	transport: Transport,
	limit: number = 5,
): Promise<RecallCandidateData[]> {
	const client = createKnowledgeHomeClient(transport);
	const response = await client.getRecallRail({ limit });
	return response.candidates.map(convertRecallCandidate);
}

export async function snoozeRecallItem(
	transport: Transport,
	itemKey: string,
	snoozeHours: number = 24,
): Promise<void> {
	const client = createKnowledgeHomeClient(transport);
	await client.trackRecallAction({
		actionType: "snooze",
		itemKey,
		snoozeHours,
	});
}

export async function dismissRecallItem(
	transport: Transport,
	itemKey: string,
): Promise<void> {
	const client = createKnowledgeHomeClient(transport);
	await client.trackRecallAction({
		actionType: "dismiss",
		itemKey,
	});
}

export async function listLenses(
	transport: Transport,
): Promise<ListLensesResult> {
	const client = createKnowledgeHomeClient(transport);
	const response = await client.listLenses({});
	return {
		lenses: response.lenses.map(convertLens),
		activeLensId: response.activeLensId || null,
	};
}

export async function createLens(
	transport: Transport,
	name: string,
	description: string,
	version: Omit<LensVersionData, "versionId">,
): Promise<LensData | null> {
	const client = createKnowledgeHomeClient(transport);
	const response = await client.createLens({
		name,
		description,
		version: {
			queryText: version.queryText,
			tagIds: version.tagIds,
			feedIds: version.feedIds,
			timeWindow: version.timeWindow,
			includeRecap: version.includeRecap,
			includePulse: version.includePulse,
			sortMode: version.sortMode,
		},
	});
	return response.lens ? convertLens(response.lens) : null;
}

export async function deleteLens(
	transport: Transport,
	lensId: string,
): Promise<void> {
	const client = createKnowledgeHomeClient(transport);
	await client.deleteLens({ lensId });
}

export async function selectLens(
	transport: Transport,
	lensId: string | null,
): Promise<void> {
	const client = createKnowledgeHomeClient(transport);
	await client.selectLens({ lensId: lensId ?? "" });
}
