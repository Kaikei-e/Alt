/**
 * KnowledgeLoopService client for Connect-RPC.
 *
 * Factory helpers for the KnowledgeLoopService introduced in ADR-000831.
 * Authentication is handled by the transport layer (server-side JWT injection).
 */

import type { Client, Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import {
	ActTargetType,
	DecisionIntent,
	DismissState,
	type GetKnowledgeLoopResponse,
	KnowledgeLoopService,
	LoopPriority,
	LoopStage,
	type KnowledgeLoopEntry as ProtoKnowledgeLoopEntry,
	type KnowledgeLoopSessionState as ProtoKnowledgeLoopSessionState,
	type SurfaceState as ProtoSurfaceState,
	RenderDepthHint,
	ServiceQuality,
	type StreamKnowledgeLoopUpdatesResponse,
	SurfaceBucket,
	SurfacePlannerVersion,
	type TransitionKnowledgeLoopResponse,
	TransitionTrigger,
	WhyKind,
} from "$lib/gen/alt/knowledge/loop/v1/knowledge_loop_pb";

export type KnowledgeLoopClient = Client<typeof KnowledgeLoopService>;

export type LoopStageName = "observe" | "orient" | "decide" | "act";
export type SurfaceBucketName = "now" | "continue" | "changed" | "review";
export type LoopPriorityName =
	| "critical"
	| "continuing"
	| "confirm"
	| "reference";
export type WhyKindName =
	| "source_why"
	| "pattern_why"
	| "recall_why"
	| "change_why"
	| "topic_affinity_why"
	| "tag_trending_why"
	| "unfinished_continue_why";
export type SurfacePlannerVersionName = "v1" | "v2";
export type DismissStateName =
	| "active"
	| "deferred"
	| "dismissed"
	| "completed";

export interface WhyPayloadData {
	kind: WhyKindName;
	text: string;
	confidence?: number;
	evidenceRefs: Array<{ refId: string; label: string }>;
}

export type DecisionIntentName =
	| "open"
	| "ask"
	| "save"
	| "compare"
	| "revisit"
	| "snooze"
	| "unspecified";

export type ActTargetTypeName =
	| "article"
	| "ask"
	| "recap"
	| "diff"
	| "cluster"
	| "unspecified";

export interface ChangeSummaryData {
	summary: string;
	changedFields: string[];
	previousEntryKey?: string;
	// Additive redline-proof diff fields. The projector populates these when
	// both old and new summary version ids resolve under the same user_id and
	// article_id; otherwise they default to empty arrays / undefined and the
	// component falls back to the legacy `summary` line.
	addedPhrases?: string[];
	removedPhrases?: string[];
	unchangedPhrasesCount?: number;
	addedTags?: string[];
	removedTags?: string[];
}

export interface ContinueContextData {
	summary: string;
	recentActionLabels: string[];
	lastInteractedAt?: string;
}

export interface DecisionOptionData {
	actionId: string;
	intent: DecisionIntentName;
	label?: string;
}

export interface ActTargetData {
	targetType: ActTargetTypeName;
	targetRef: string;
	/**
	 * Internal SPA navigation target (e.g. "/articles/<id>", "/recap/topic/<id>").
	 * Display-only; never threaded through `?url=` to the SPA reader.
	 */
	route?: string;
	/**
	 * External HTTPS source URL when targetType is "article".
	 * Used by the SPA reader as `?url=` (see ADR for source_url decision).
	 * Independent of `route`; absent for legacy projection rows.
	 */
	sourceUrl?: string;
}

export interface KnowledgeLoopEntryData {
	entryKey: string;
	sourceItemKey: string;
	proposedStage: LoopStageName;
	currentEntryStage?: LoopStageName;
	currentEntryStageEnteredAt?: string;
	surfaceBucket: SurfaceBucketName;
	projectionRevision: number;
	projectionSeqHiwater: number;
	freshnessAt: string;
	sourceObservedAt?: string;
	whyPrimary: WhyPayloadData;
	dismissState: DismissStateName;
	renderDepthHint: 1 | 2 | 3 | 4;
	loopPriority: LoopPriorityName;
	surfacePlannerVersion?: SurfacePlannerVersionName;
	supersededByEntryKey?: string;
	changeSummary?: ChangeSummaryData;
	continueContext?: ContinueContextData;
	decisionOptions: DecisionOptionData[];
	actTargets: ActTargetData[];
}

export interface KnowledgeLoopSessionStateData {
	currentStage: LoopStageName;
	currentStageEnteredAt: string;
	foregroundEntryKey?: string;
	focusedEntryKey?: string;
	projectionRevision: number;
	projectionSeqHiwater: number;
}

export interface SurfaceStateData {
	surfaceBucket: SurfaceBucketName;
	primaryEntryKey?: string;
	secondaryEntryKeys: string[];
	projectionRevision: number;
	projectionSeqHiwater: number;
	freshnessAt: string;
	serviceQuality: "full" | "degraded" | "fallback" | "unspecified";
}

export interface KnowledgeLoopResult {
	foregroundEntries: KnowledgeLoopEntryData[];
	/**
	 * Entries for the non-NOW buckets (Continue / Changed / Review). Each entry
	 * carries its `surfaceBucket` so callers partition into planes.
	 * Empty when the server is an older build that does not populate bucket_entries
	 * yet; the `/loop` page falls back to the legacy bucket-index view in that case.
	 */
	bucketEntries: KnowledgeLoopEntryData[];
	surfaces: SurfaceStateData[];
	sessionState?: KnowledgeLoopSessionStateData;
	overallServiceQuality: "full" | "degraded" | "fallback" | "unspecified";
	generatedAt: string;
	projectionSeqHiwater: number;
}

/** Create a strongly-typed Connect-RPC client for the Knowledge Loop service. */
export function createKnowledgeLoopClient(
	transport: Transport,
): KnowledgeLoopClient {
	return createClient(KnowledgeLoopService, transport);
}

/** Fetch the Loop read model for the authenticated user. */
export async function getKnowledgeLoop(
	transport: Transport,
	lensModeId: string,
	opts: { foregroundLimit?: number; reducedMotion?: boolean } = {},
): Promise<KnowledgeLoopResult> {
	const client = createKnowledgeLoopClient(transport);
	const resp = await client.getKnowledgeLoop({
		lensModeId,
		foregroundLimit: opts.foregroundLimit,
		reducedMotion: opts.reducedMotion,
	});
	return mapGetKnowledgeLoopResponse(resp);
}

/** Record a Loop session stage transition (with client-generated UUIDv7 idempotency key). */
export async function transitionKnowledgeLoop(
	transport: Transport,
	args: {
		lensModeId: string;
		clientTransitionId: string;
		entryKey: string;
		fromStage: LoopStageName;
		toStage: LoopStageName;
		// `defer` routes to KnowledgeLoopDeferred (canonical contract §8.2 — soft
		// dismiss / snooze). `recheck` / `archive` / `mark_reviewed` are the
		// Review-lane re-evaluation triggers (fb.md §F) that route to
		// KnowledgeLoopReviewed. All four require fromStage === toStage.
		trigger:
			| "user_tap"
			| "dwell"
			| "keyboard"
			| "programmatic"
			| "defer"
			| "recheck"
			| "archive"
			| "mark_reviewed";
		observedProjectionRevision: number;
		presentedIntents?: DecisionIntentName[];
		actedIntent?: DecisionIntentName;
		actionId?: string;
		targetType?: ActTargetTypeName;
		targetRef?: string;
		continueFlag?: boolean;
	},
): Promise<TransitionKnowledgeLoopResponse> {
	const client = createKnowledgeLoopClient(transport);
	return client.transitionKnowledgeLoop({
		lensModeId: args.lensModeId,
		clientTransitionId: args.clientTransitionId,
		entryKey: args.entryKey,
		fromStage: mapStageToProto(args.fromStage),
		toStage: mapStageToProto(args.toStage),
		trigger: mapTriggerToProto(args.trigger),
		observedProjectionRevision: BigInt(args.observedProjectionRevision),
		presentedIntents: args.presentedIntents?.map(mapDecisionIntentToProto),
		actedIntent: args.actedIntent
			? mapDecisionIntentToProto(args.actedIntent)
			: undefined,
		actionId: args.actionId,
		targetType: args.targetType
			? mapActTargetTypeToProto(args.targetType)
			: undefined,
		targetRef: args.targetRef,
		continueFlag: args.continueFlag === true,
	});
}

function mapGetKnowledgeLoopResponse(
	resp: GetKnowledgeLoopResponse,
): KnowledgeLoopResult {
	return {
		foregroundEntries: resp.foregroundEntries.map(mapProtoEntry),
		bucketEntries: (resp.bucketEntries ?? []).map(mapProtoEntry),
		surfaces: resp.surfaces.map(mapProtoSurface),
		sessionState: resp.sessionState
			? mapProtoSession(resp.sessionState)
			: undefined,
		overallServiceQuality: mapServiceQuality(resp.overallServiceQuality),
		generatedAt: tsToIso(resp.generatedAt),
		projectionSeqHiwater: Number(resp.projectionSeqHiwater),
	};
}

export function mapProtoEntry(
	e: ProtoKnowledgeLoopEntry,
): KnowledgeLoopEntryData {
	return {
		entryKey: e.entryKey,
		sourceItemKey: e.sourceItemKey,
		proposedStage: mapStageFromProto(e.proposedStage),
		currentEntryStage:
			e.currentEntryStage !== undefined
				? mapStageFromProto(e.currentEntryStage)
				: undefined,
		currentEntryStageEnteredAt: e.currentEntryStageEnteredAt
			? tsToIso(e.currentEntryStageEnteredAt)
			: undefined,
		surfaceBucket: mapBucketFromProto(e.surfaceBucket),
		projectionRevision: Number(e.projectionRevision),
		projectionSeqHiwater: Number(e.projectionSeqHiwater),
		freshnessAt: tsToIso(e.freshnessAt),
		sourceObservedAt: e.sourceObservedAt
			? tsToIso(e.sourceObservedAt)
			: undefined,
		whyPrimary: {
			kind: mapWhyKindFromProto(e.whyPrimary?.kind),
			text: e.whyPrimary?.text ?? "",
			confidence: e.whyPrimary?.confidence,
			evidenceRefs:
				e.whyPrimary?.evidenceRefs.map((r) => ({
					refId: r.refId,
					label: r.label,
				})) ?? [],
		},
		dismissState: mapDismissFromProto(e.dismissState),
		renderDepthHint: mapDepthHintFromProto(e.renderDepthHint),
		loopPriority: mapPriorityFromProto(e.loopPriority),
		surfacePlannerVersion: mapSurfacePlannerVersionFromProto(
			e.surfacePlannerVersion,
		),
		supersededByEntryKey: e.supersededByEntryKey,
		changeSummary: e.changeSummary
			? {
					summary: e.changeSummary.summary,
					changedFields: [...e.changeSummary.changedFields],
					previousEntryKey: e.changeSummary.previousEntryKey,
					addedPhrases:
						e.changeSummary.addedPhrases &&
						e.changeSummary.addedPhrases.length > 0
							? [...e.changeSummary.addedPhrases]
							: undefined,
					removedPhrases:
						e.changeSummary.removedPhrases &&
						e.changeSummary.removedPhrases.length > 0
							? [...e.changeSummary.removedPhrases]
							: undefined,
					unchangedPhrasesCount: e.changeSummary.unchangedPhrasesCount,
					addedTags:
						e.changeSummary.addedTags && e.changeSummary.addedTags.length > 0
							? [...e.changeSummary.addedTags]
							: undefined,
					removedTags:
						e.changeSummary.removedTags &&
						e.changeSummary.removedTags.length > 0
							? [...e.changeSummary.removedTags]
							: undefined,
				}
			: undefined,
		continueContext: e.continueContext
			? {
					summary: e.continueContext.summary,
					recentActionLabels: [...e.continueContext.recentActionLabels],
					lastInteractedAt: e.continueContext.lastInteractedAt
						? tsToIso(e.continueContext.lastInteractedAt)
						: undefined,
				}
			: undefined,
		decisionOptions: (e.decisionOptions ?? []).map((o) => ({
			actionId: o.actionId,
			intent: mapDecisionIntentFromProto(o.intent),
			label: o.label,
		})),
		actTargets: (e.actTargets ?? []).map((t) => ({
			targetType: mapActTargetTypeFromProto(t.targetType),
			targetRef: t.targetRef,
			route: t.route,
			sourceUrl: t.sourceUrl,
		})),
	};
}

function mapProtoSurface(s: ProtoSurfaceState): SurfaceStateData {
	return {
		surfaceBucket: mapBucketFromProto(s.surfaceBucket),
		primaryEntryKey: s.primaryEntryKey,
		secondaryEntryKeys: [...s.secondaryEntryKeys],
		projectionRevision: Number(s.projectionRevision),
		projectionSeqHiwater: Number(s.projectionSeqHiwater),
		freshnessAt: tsToIso(s.freshnessAt),
		serviceQuality: mapServiceQuality(s.serviceQuality),
	};
}

function mapProtoSession(
	s: ProtoKnowledgeLoopSessionState,
): KnowledgeLoopSessionStateData {
	return {
		currentStage: mapStageFromProto(s.currentStage),
		currentStageEnteredAt: tsToIso(s.currentStageEnteredAt),
		foregroundEntryKey: s.foregroundEntryKey,
		focusedEntryKey: s.focusedEntryKey,
		projectionRevision: Number(s.projectionRevision),
		projectionSeqHiwater: Number(s.projectionSeqHiwater),
	};
}

function tsToIso(ts: { seconds: bigint; nanos: number } | undefined): string {
	if (!ts) return "";
	const ms = Number(ts.seconds) * 1000 + Math.floor(ts.nanos / 1_000_000);
	return new Date(ms).toISOString();
}

function mapStageFromProto(s: LoopStage): LoopStageName {
	switch (s) {
		case LoopStage.OBSERVE:
			return "observe";
		case LoopStage.ORIENT:
			return "orient";
		case LoopStage.DECIDE:
			return "decide";
		case LoopStage.ACT:
			return "act";
		default:
			return "observe";
	}
}

function mapStageToProto(s: LoopStageName): LoopStage {
	switch (s) {
		case "observe":
			return LoopStage.OBSERVE;
		case "orient":
			return LoopStage.ORIENT;
		case "decide":
			return LoopStage.DECIDE;
		case "act":
			return LoopStage.ACT;
	}
}

function mapBucketFromProto(b: SurfaceBucket): SurfaceBucketName {
	switch (b) {
		case SurfaceBucket.NOW:
			return "now";
		case SurfaceBucket.CONTINUE:
			return "continue";
		case SurfaceBucket.CHANGED:
			return "changed";
		case SurfaceBucket.REVIEW:
			return "review";
		default:
			return "now";
	}
}

function mapDismissFromProto(d: DismissState): DismissStateName {
	switch (d) {
		case DismissState.ACTIVE:
			return "active";
		case DismissState.DEFERRED:
			return "deferred";
		case DismissState.DISMISSED:
			return "dismissed";
		case DismissState.COMPLETED:
			return "completed";
		default:
			return "active";
	}
}

function mapDepthHintFromProto(d: RenderDepthHint): 1 | 2 | 3 | 4 {
	switch (d) {
		case RenderDepthHint.FLAT:
			return 1;
		case RenderDepthHint.LIGHT:
			return 2;
		case RenderDepthHint.STRONG:
			return 3;
		case RenderDepthHint.CRITICAL:
			return 4;
		default:
			return 1;
	}
}

function mapPriorityFromProto(p: LoopPriority): LoopPriorityName {
	switch (p) {
		case LoopPriority.CRITICAL:
			return "critical";
		case LoopPriority.CONTINUING:
			return "continuing";
		case LoopPriority.CONFIRM:
			return "confirm";
		case LoopPriority.REFERENCE:
			return "reference";
		default:
			return "reference";
	}
}

function mapDecisionIntentFromProto(i: DecisionIntent): DecisionIntentName {
	switch (i) {
		case DecisionIntent.OPEN:
			return "open";
		case DecisionIntent.ASK:
			return "ask";
		case DecisionIntent.SAVE:
			return "save";
		case DecisionIntent.COMPARE:
			return "compare";
		case DecisionIntent.REVISIT:
			return "revisit";
		case DecisionIntent.SNOOZE:
			return "snooze";
		default:
			return "unspecified";
	}
}

function mapDecisionIntentToProto(i: DecisionIntentName): DecisionIntent {
	switch (i) {
		case "open":
			return DecisionIntent.OPEN;
		case "ask":
			return DecisionIntent.ASK;
		case "save":
			return DecisionIntent.SAVE;
		case "compare":
			return DecisionIntent.COMPARE;
		case "revisit":
			return DecisionIntent.REVISIT;
		case "snooze":
			return DecisionIntent.SNOOZE;
		default:
			return DecisionIntent.UNSPECIFIED;
	}
}

function mapActTargetTypeFromProto(t: ActTargetType): ActTargetTypeName {
	switch (t) {
		case ActTargetType.ARTICLE:
			return "article";
		case ActTargetType.ASK:
			return "ask";
		case ActTargetType.RECAP:
			return "recap";
		case ActTargetType.DIFF:
			return "diff";
		case ActTargetType.CLUSTER:
			return "cluster";
		default:
			return "unspecified";
	}
}

function mapActTargetTypeToProto(t: ActTargetTypeName): ActTargetType {
	switch (t) {
		case "article":
			return ActTargetType.ARTICLE;
		case "ask":
			return ActTargetType.ASK;
		case "recap":
			return ActTargetType.RECAP;
		case "diff":
			return ActTargetType.DIFF;
		case "cluster":
			return ActTargetType.CLUSTER;
		default:
			return ActTargetType.UNSPECIFIED;
	}
}

function mapWhyKindFromProto(k: WhyKind | undefined): WhyKindName {
	switch (k) {
		case WhyKind.SOURCE:
			return "source_why";
		case WhyKind.PATTERN:
			return "pattern_why";
		case WhyKind.RECALL:
			return "recall_why";
		case WhyKind.CHANGE:
			return "change_why";
		case WhyKind.TOPIC_AFFINITY:
			return "topic_affinity_why";
		case WhyKind.TAG_TRENDING:
			return "tag_trending_why";
		case WhyKind.UNFINISHED_CONTINUE:
			return "unfinished_continue_why";
		default:
			return "source_why";
	}
}

function mapSurfacePlannerVersionFromProto(
	v: SurfacePlannerVersion | undefined,
): SurfacePlannerVersionName {
	if (v === SurfacePlannerVersion.V2) return "v2";
	return "v1";
}

function mapServiceQuality(
	q: ServiceQuality,
): "full" | "degraded" | "fallback" | "unspecified" {
	switch (q) {
		case ServiceQuality.FULL:
			return "full";
		case ServiceQuality.DEGRADED:
			return "degraded";
		case ServiceQuality.FALLBACK:
			return "fallback";
		default:
			return "unspecified";
	}
}

function mapTriggerToProto(
	t:
		| "user_tap"
		| "dwell"
		| "keyboard"
		| "programmatic"
		| "defer"
		| "recheck"
		| "archive"
		| "mark_reviewed",
): TransitionTrigger {
	switch (t) {
		case "user_tap":
			return TransitionTrigger.USER_TAP;
		case "dwell":
			return TransitionTrigger.DWELL;
		case "keyboard":
			return TransitionTrigger.KEYBOARD;
		case "programmatic":
			return TransitionTrigger.PROGRAMMATIC;
		case "defer":
			return TransitionTrigger.DEFER;
		case "recheck":
			return TransitionTrigger.RECHECK;
		case "archive":
			return TransitionTrigger.ARCHIVE;
		case "mark_reviewed":
			return TransitionTrigger.MARK_REVIEWED;
	}
}

export type { StreamKnowledgeLoopUpdatesResponse };
