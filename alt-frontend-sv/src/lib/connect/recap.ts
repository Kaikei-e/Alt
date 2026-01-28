/**
 * RecapService client for Connect-RPC
 *
 * Provides type-safe methods to call RecapService endpoints.
 * Authentication is handled by the transport layer.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	RecapService,
	type GetSevenDayRecapResponse,
} from "$lib/gen/alt/recap/v2/recap_pb";
import type { RecapSummary, RecapGenre, EvidenceLink } from "$lib/schema/recap";

/** Type-safe RecapService client */
type RecapClient = Client<typeof RecapService>;

/**
 * Reference type from the recap response
 */
export interface RecapReference {
	id: number;
	url: string;
	domain: string;
	articleId?: string;
}

/**
 * Extended RecapGenre with references (from Connect-RPC response)
 */
export interface RecapGenreWithReferences extends RecapGenre {
	references: RecapReference[];
}

/**
 * Extended RecapSummary with references support (from Connect-RPC response)
 */
export interface RecapSummaryWithReferences extends Omit<RecapSummary, "genres"> {
	genres: RecapGenreWithReferences[];
}

/**
 * Creates a RecapService client with the given transport.
 */
export function createRecapClient(transport: Transport): RecapClient {
	return createClient(RecapService, transport);
}

/**
 * Gets 7-day recap summary via Connect-RPC.
 *
 * @param transport - The Connect transport to use (must include auth)
 * @param genreDraftId - Optional draft ID for cluster draft attachment
 * @returns Recap summary with genres
 */
export async function getSevenDayRecap(
	transport: Transport,
	genreDraftId?: string,
): Promise<RecapSummaryWithReferences> {
	const client = createRecapClient(transport);
	const response = (await client.getSevenDayRecap({
		genreDraftId,
	})) as GetSevenDayRecapResponse;

	return {
		jobId: response.jobId,
		executedAt: response.executedAt,
		windowStart: response.windowStart,
		windowEnd: response.windowEnd,
		totalArticles: response.totalArticles,
		genres: response.genres.map(convertProtoGenre),
	};
}

/**
 * Convert proto RecapGenre to frontend type.
 */
function convertProtoGenre(proto: {
	genre: string;
	summary: string;
	topTerms: string[];
	articleCount: number;
	clusterCount: number;
	evidenceLinks: Array<{
		articleId: string;
		title: string;
		sourceUrl: string;
		publishedAt: string;
		lang: string;
	}>;
	bullets: string[];
	references: Array<{
		id: number;
		url: string;
		domain: string;
		articleId?: string;
	}>;
}): RecapGenreWithReferences {
	return {
		genre: proto.genre,
		summary: proto.summary,
		topTerms: proto.topTerms,
		articleCount: proto.articleCount,
		clusterCount: proto.clusterCount,
		evidenceLinks: proto.evidenceLinks.map(convertProtoEvidenceLink),
		bullets: proto.bullets,
		references: proto.references.map(convertProtoReference),
	};
}

/**
 * Convert proto EvidenceLink to frontend type.
 */
function convertProtoEvidenceLink(proto: {
	articleId: string;
	title: string;
	sourceUrl: string;
	publishedAt: string;
	lang: string;
}): EvidenceLink {
	return {
		articleId: proto.articleId,
		title: proto.title,
		sourceUrl: proto.sourceUrl,
		publishedAt: proto.publishedAt,
		lang: proto.lang,
	};
}

/**
 * Convert proto Reference to frontend type.
 */
function convertProtoReference(proto: {
	id: number;
	url: string;
	domain: string;
	articleId?: string;
}): RecapReference {
	return {
		id: proto.id,
		url: proto.url,
		domain: proto.domain,
		articleId: proto.articleId || undefined,
	};
}
