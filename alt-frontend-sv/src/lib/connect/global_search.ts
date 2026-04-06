/**
 * GlobalSearchService client for Connect-RPC
 *
 * Provides type-safe methods to call GlobalSearchService endpoints.
 * Authentication is handled by the transport layer.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	GlobalSearchService,
	type SearchEverythingResponse,
	type ArticleSection as ProtoArticleSection,
	type RecapSection as ProtoRecapSection,
	type TagSection as ProtoTagSection,
	type GlobalArticleHit as ProtoGlobalArticleHit,
	type GlobalRecapHit as ProtoGlobalRecapHit,
	type GlobalTagHit as ProtoGlobalTagHit,
} from "$lib/gen/alt/search/v2/global_search_pb";

/** Type-safe GlobalSearchService client */
type GlobalSearchClient = Client<typeof GlobalSearchService>;

/** A single article search result */
export interface GlobalArticleHitData {
	id: string;
	title: string;
	snippet: string;
	link: string;
	tags: string[];
	matchedFields: string[];
}

/** A single recap search result */
export interface GlobalRecapHitData {
	id: string;
	jobId: string;
	genre: string;
	summary: string;
	topTerms: string[];
	tags: string[];
	windowDays: number;
	executedAt: string;
}

/** A single tag search result */
export interface GlobalTagHitData {
	tagName: string;
	articleCount: number;
}

/** Article section with hits and pagination */
export interface ArticleSectionData {
	hits: GlobalArticleHitData[];
	estimatedTotal: number;
	hasMore: boolean;
}

/** Recap section with hits and pagination */
export interface RecapSectionData {
	hits: GlobalRecapHitData[];
	estimatedTotal: number;
	hasMore: boolean;
}

/** Tag section with hits */
export interface TagSectionData {
	hits: GlobalTagHitData[];
	total: number;
}

/** Result from searchEverything */
export interface GlobalSearchResult {
	query: string;
	articleSection: ArticleSectionData | null;
	recapSection: RecapSectionData | null;
	tagSection: TagSectionData | null;
	degradedSections: string[];
	searchedAt: string;
}

/** Optional search limits */
export interface GlobalSearchOptions {
	articleLimit?: number;
	recapLimit?: number;
	tagLimit?: number;
}

/**
 * Creates a GlobalSearchService client with the given transport.
 */
export function createGlobalSearchClient(
	transport: Transport,
): GlobalSearchClient {
	return createClient(GlobalSearchService, transport);
}

function convertArticleHit(proto: ProtoGlobalArticleHit): GlobalArticleHitData {
	return {
		id: proto.id,
		title: proto.title,
		snippet: proto.snippet,
		link: proto.link,
		tags: [...proto.tags],
		matchedFields: [...proto.matchedFields],
	};
}

function convertRecapHit(proto: ProtoGlobalRecapHit): GlobalRecapHitData {
	return {
		id: proto.id,
		jobId: proto.jobId,
		genre: proto.genre,
		summary: proto.summary,
		topTerms: [...proto.topTerms],
		tags: [...proto.tags],
		windowDays: proto.windowDays,
		executedAt: proto.executedAt,
	};
}

function convertTagHit(proto: ProtoGlobalTagHit): GlobalTagHitData {
	return {
		tagName: proto.tagName,
		articleCount: proto.articleCount,
	};
}

function convertArticleSection(
	proto: ProtoArticleSection | undefined,
): ArticleSectionData | null {
	if (!proto) return null;
	return {
		hits: proto.hits.map(convertArticleHit),
		estimatedTotal: Number(proto.estimatedTotal),
		hasMore: proto.hasMore,
	};
}

function convertRecapSection(
	proto: ProtoRecapSection | undefined,
): RecapSectionData | null {
	if (!proto) return null;
	return {
		hits: proto.hits.map(convertRecapHit),
		estimatedTotal: Number(proto.estimatedTotal),
		hasMore: proto.hasMore,
	};
}

function convertTagSection(
	proto: ProtoTagSection | undefined,
): TagSectionData | null {
	if (!proto) return null;
	return {
		hits: proto.hits.map(convertTagHit),
		total: Number(proto.total),
	};
}

/**
 * Performs a federated search across all content verticals.
 *
 * @param transport - The Connect transport to use
 * @param query - Search query string
 * @param options - Optional limits for each section
 * @returns Federated search results with article, recap, and tag sections
 */
export async function searchEverything(
	transport: Transport,
	query: string,
	options?: GlobalSearchOptions,
): Promise<GlobalSearchResult> {
	const client = createGlobalSearchClient(transport);
	const response = (await client.searchEverything({
		query,
		articleLimit: options?.articleLimit ?? 0,
		recapLimit: options?.recapLimit ?? 0,
		tagLimit: options?.tagLimit ?? 0,
	})) as SearchEverythingResponse;

	return {
		query: response.query,
		articleSection: convertArticleSection(response.articleSection),
		recapSection: convertRecapSection(response.recapSection),
		tagSection: convertTagSection(response.tagSection),
		degradedSections: [...response.degradedSections],
		searchedAt: response.searchedAt,
	};
}
