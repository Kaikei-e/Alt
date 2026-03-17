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
} from "$lib/gen/alt/knowledge_home/v1/knowledge_home_pb";

/** Type-safe KnowledgeHomeService client */
type KnowledgeHomeClient = Client<typeof KnowledgeHomeService>;

/** Why reason explaining why an item appeared */
export interface WhyReasonData {
	code: string;
	refId?: string;
	tag?: string;
}

/** Today's digest summary */
export interface TodayDigestData {
	date: string;
	newArticles: number;
	summarizedArticles: number;
	unsummarizedArticles: number;
	topTags: string[];
	weeklyRecapAvailable: boolean;
	eveningPulseAvailable: boolean;
}

/** A single Knowledge Home item */
export interface KnowledgeHomeItemData {
	itemKey: string;
	itemType: string;
	articleId?: string;
	recapId?: string;
	title: string;
	publishedAt: string;
	summaryExcerpt?: string;
	tags: string[];
	why: WhyReasonData[];
	score: number;
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
	};
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
		tags: [...proto.tags],
		why: proto.why.map(convertWhyReason),
		score: proto.score,
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
): Promise<KnowledgeHomeResult> {
	const client = createKnowledgeHomeClient(transport);
	const response = (await client.getKnowledgeHome({
		cursor,
		limit,
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
