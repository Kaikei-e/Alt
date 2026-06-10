/**
 * KnowledgeTrailService client for Connect-RPC.
 *
 * Provides type-safe access to the Knowledge Trail footprint spine.
 * Authentication is handled by the transport layer.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	KnowledgeTrailService,
	type GetTrailResponse,
	type Footprint as ProtoFootprint,
	type Branch as ProtoBranch,
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
	occurredAt: string;
	wear: FootprintWear;
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

/** One page of the trail spine. */
export interface TrailResult {
	footprints: FootprintData[];
	branches: BranchData[];
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
		nextCursor: response.nextCursor,
		hasMore: response.hasMore,
	};
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
