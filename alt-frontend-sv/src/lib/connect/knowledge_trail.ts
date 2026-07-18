/**
 * KnowledgeTrailService client for Connect-RPC.
 *
 * Provides type-safe access to the Knowledge Trail footprint spine.
 * Authentication is handled by the transport layer.
 */

import type { Client, Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import {
	type GetTrailResponse,
	KnowledgeTrailService,
	type Branch as ProtoBranch,
	type Episode as ProtoEpisode,
	type Footprint as ProtoFootprint,
	type SearchTrailResponse,
} from "$lib/gen/alt/knowledge_trail/v1/knowledge_trail_pb";

type KnowledgeTrailClient = Client<typeof KnowledgeTrailService>;

/** The user-facing action a footprint represents. */
export type FootprintVerb =
	| "read"
	| "asked"
	| "returned"
	| "listened"
	| "dismissed"
	| string;

/** Derived path-wear band — never a number. */
export type FootprintWear = "thin" | "worn" | "deep" | string;

/** One footprint on the trail spine. */
export interface FootprintData {
	footprintKey: string;
	verb: FootprintVerb;
	itemKey: string;
	title: string;
	excerpt: string;
	tags: string[];
	note: string;
	/** Event-time of the LATEST collapsed contact (RFC3339). */
	occurredAt: string;
	wear: FootprintWear;
	/** How many contacts collapse into this row (>= 1). Repeated reads never add rows. */
	contactCount: number;
	/** Event-time of the EARLIEST collapsed contact (RFC3339). */
	firstOccurredAt: string;
}

/** The typed relation a branch expresses. */
export type BranchRelationKind =
	| "continuation"
	| "cluster"
	| "contradiction"
	| "inquiry"
	| string;

/** One piece of evidence backing a branch. */
export interface EvidenceRefData {
	refId: string;
	label: string;
	kind: string;
}

/** A system-proposed next step on the trail. Always carries the four-tuple. */
export interface BranchData {
	branchKey: string;
	anchorItemKey: string;
	relationKind: BranchRelationKind;
	why: string;
	evidenceRefs: EvidenceRefData[];
	confidence: string;
	targetItemKey: string;
	targetTitle: string;
}

/**
 * One derived "line of inquiry" (D24/D30): footprints folded by same-article
 * contact and cleaned-tag chaining. A pure derivation over the spine, never a
 * second spine — expanding it returns to plain footprints.
 */
export interface EpisodeData {
	episodeKey: string;
	/** Deepest path-wear band among member footprints. */
	wear: FootprintWear;
	/** Representative article's preview image ("" falls back to text). */
	thumbnailUrl: string;
	/** Member rows, newest first (collapsed per D24). */
	footprints: FootprintData[];
}

/** One page of the trail spine. */
export interface TrailResult {
	/** Legacy flat spine (superseded by episodes; empty once episodes ship). */
	footprints: FootprintData[];
	branches: BranchData[];
	/** The spine's default display unit (Wave 8, D24). */
	episodes: EpisodeData[];
	nextCursor: string;
	hasMore: boolean;
}

export function createKnowledgeTrailClient(
	transport: Transport,
): KnowledgeTrailClient {
	return createClient(KnowledgeTrailService, transport);
}

function convertFootprint(pb: ProtoFootprint): FootprintData {
	return {
		footprintKey: pb.footprintKey,
		verb: pb.verb,
		itemKey: pb.itemKey,
		title: pb.title,
		excerpt: pb.excerpt,
		tags: pb.tags ?? [],
		note: pb.note,
		occurredAt: pb.occurredAt,
		wear: pb.wear,
		contactCount: pb.contactCount > 0 ? pb.contactCount : 1,
		firstOccurredAt: pb.firstOccurredAt || pb.occurredAt,
	};
}

function convertEpisode(pb: ProtoEpisode): EpisodeData {
	return {
		episodeKey: pb.episodeKey,
		wear: pb.wear,
		thumbnailUrl: pb.thumbnailUrl,
		footprints: pb.footprints.map(convertFootprint),
	};
}

/**
 * Fetches one page of the user's footprint spine, reverse-chronological.
 * `filterTags` applies the theme lens (empty = full spine).
 */
export async function getTrail(
	transport: Transport,
	cursor?: string,
	limit = 20,
	filterTags: string[] = [],
): Promise<TrailResult> {
	const client = createKnowledgeTrailClient(transport);
	const response = (await client.getTrail({
		cursor,
		limit,
		filterTags,
	})) as GetTrailResponse;

	return {
		footprints: response.footprints.map(convertFootprint),
		branches: response.branches.map(convertBranch),
		episodes: response.episodes.map(convertEpisode),
		nextCursor: response.nextCursor,
		hasMore: response.hasMore,
	};
}

/** Result of a trail search (D25): matching episodes plus which member items hit. */
export interface SearchTrailResult {
	/** Episodes containing at least one matching item, newest first. */
	episodes: EpisodeData[];
	/** Member item keys that matched, so the UI can highlight the hit. */
	matchedItemKeys: string[];
}

/**
 * Searches the user's trail (D25): full-text over what was actually read,
 * intersected with the spine. Hits return as their containing episodes so
 * every result keeps its time context. Pull-only — call only on explicit
 * submit, never on keystroke.
 */
export async function searchTrail(
	transport: Transport,
	query: string,
	limit = 20,
): Promise<SearchTrailResult> {
	const client = createKnowledgeTrailClient(transport);
	const response = (await client.searchTrail({
		query,
		limit,
	})) as SearchTrailResponse;

	return {
		episodes: response.episodes.map(convertEpisode),
		matchedItemKeys: response.matchedItemKeys ?? [],
	};
}

/** How a user resolved a proposed branch. */
export type BranchResolution = "taken" | "dismissed";

/**
 * Records the user's resolution of a branch. `clientResolutionId` must be a
 * UUIDv7 so retries are idempotent server-side.
 */
export async function resolveBranch(
	transport: Transport,
	branchKey: string,
	resolution: BranchResolution,
	clientResolutionId: string,
): Promise<void> {
	const client = createKnowledgeTrailClient(transport);
	await client.resolveBranch({ branchKey, resolution, clientResolutionId });
}

/**
 * Records the raw dwell observed after a taken branch. Idempotent per branch —
 * the server dedupes on the branch key (one outcome per proposal, first write
 * wins), so retries need no client-minted id.
 */
export async function emitTrailOutcome(
	transport: Transport,
	branchKey: string,
	itemKey: string,
	dwellMs: bigint,
): Promise<void> {
	const client = createKnowledgeTrailClient(transport);
	await client.emitTrailOutcome({ branchKey, itemKey, dwellMs });
}

function convertBranch(pb: ProtoBranch): BranchData {
	return {
		branchKey: pb.branchKey,
		anchorItemKey: pb.anchorItemKey,
		relationKind: pb.relationKind,
		why: pb.why,
		evidenceRefs: (pb.evidenceRefs ?? []).map((r) => ({
			refId: r.refId,
			label: r.label,
			kind: r.kind,
		})),
		confidence: pb.confidence,
		targetItemKey: pb.targetItemKey,
		targetTitle: pb.targetTitle,
	};
}
