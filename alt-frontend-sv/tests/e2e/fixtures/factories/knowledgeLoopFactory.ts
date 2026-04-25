/**
 * Factory for Knowledge Loop E2E test data.
 *
 * Returns Connect-RPC JSON shapes matching `GetKnowledgeLoopResponse`
 * (alt.knowledge.loop.v1). Enum values are numeric to match the Connect-ES
 * JSON wire format used by the SvelteKit BFF transport. Timestamps are ISO
 * strings, which Connect-JSON accepts for `google.protobuf.Timestamp`.
 *
 * The fixtures here are designed to *catch the regression that ADR-000844
 * fixed*: the user reported every card showing "New summary" with only the
 * Ask CTA functional. The base `LOOP_ENTRY_OBSERVE_FRESH` entry uses a
 * substantive narrative + Observe-stage seed (revisit/ask/snooze), so the
 * E2E spec asserts the contract — not the placeholder behavior.
 */

const MS_PER_HOUR = 3_600_000;

const occurredAtIso = (offsetMs: number): string =>
	new Date(Date.now() - offsetMs).toISOString();

// Loop stage enum (numeric; matches generated proto enum)
const LOOP_STAGE_OBSERVE = 1;

// Surface bucket enum
const SURFACE_BUCKET_NOW = 1;

// Dismiss state
const DISMISS_STATE_ACTIVE = 1;

// Loop priority
const LOOP_PRIORITY_CRITICAL = 1;

// Why kind
const WHY_KIND_SOURCE = 1;

// Decision intent
const DECISION_INTENT_REVISIT = 5;
const DECISION_INTENT_ASK = 2;
const DECISION_INTENT_SNOOZE = 6;

// Service quality
const SERVICE_QUALITY_FULL = 1;

export const LOOP_ENTRY_OBSERVE_FRESH = {
	entryKey: "article:e2e-loop-fresh-1",
	sourceItemKey: "article:e2e-loop-fresh-1",
	proposedStage: LOOP_STAGE_OBSERVE,
	surfaceBucket: SURFACE_BUCKET_NOW,
	projectionRevision: 1,
	projectionSeqHiwater: 1183646,
	sourceEventSeq: 1183646,
	freshnessAt: occurredAtIso(3 * MS_PER_HOUR),
	whyPrimary: {
		kind: WHY_KIND_SOURCE,
		text: "How Event Sourcing Changes Everything — fresh summary ready to read.",
		evidenceRefs: [
			{ refId: "sv-fresh-1", label: "summary" },
			{ refId: "article:e2e-loop-fresh-1", label: "article" },
		],
	},
	artifactVersionRef: {
		summaryVersionId: "sv-fresh-1",
	},
	dismissState: DISMISS_STATE_ACTIVE,
	renderDepthHint: 4,
	loopPriority: LOOP_PRIORITY_CRITICAL,
	// Stage-appropriate CTAs per ADR-000844 / canonical contract §7.
	// Observe → orient via revisit; ask + snooze are non-transition CTAs.
	// Earlier seeds emitted open/save/snooze (all → act), which §7 forbids
	// from observe and which the FE rendered as disabled buttons.
	decisionOptions: [
		{ actionId: "revisit", intent: DECISION_INTENT_REVISIT },
		{ actionId: "ask", intent: DECISION_INTENT_ASK },
		{ actionId: "snooze", intent: DECISION_INTENT_SNOOZE },
	],
	actTargets: [],
};

export const LOOP_ENTRY_OBSERVE_NO_TITLE = {
	...LOOP_ENTRY_OBSERVE_FRESH,
	entryKey: "article:e2e-loop-fresh-2",
	sourceItemKey: "article:e2e-loop-fresh-2",
	whyPrimary: {
		kind: WHY_KIND_SOURCE,
		// Fallback narrative when the event payload lacks article_title.
		// Must NEVER be the literal "New summary" — that placeholder is the
		// regression ADR-000844 closed.
		text: "A new summary is ready in one of your feeds.",
		evidenceRefs: [{ refId: "sv-fresh-2", label: "summary" }],
	},
};

export const SESSION_STATE_OBSERVE = {
	currentStage: LOOP_STAGE_OBSERVE,
	currentStageEnteredAt: occurredAtIso(10 * 60_000),
	projectionRevision: 1,
	projectionSeqHiwater: 1183646,
};

export interface BuildLoopResponseOpts {
	foreground?: Array<typeof LOOP_ENTRY_OBSERVE_FRESH>;
	bucket?: Array<typeof LOOP_ENTRY_OBSERVE_FRESH>;
	sessionState?: typeof SESSION_STATE_OBSERVE | null;
}

export function buildGetKnowledgeLoopResponse(
	opts: BuildLoopResponseOpts = {},
) {
	return {
		foregroundEntries: opts.foreground ?? [LOOP_ENTRY_OBSERVE_FRESH],
		bucketEntries: opts.bucket ?? [],
		surfaces: [],
		sessionState: opts.sessionState ?? SESSION_STATE_OBSERVE,
		overallServiceQuality: SERVICE_QUALITY_FULL,
		generatedAt: occurredAtIso(0),
		projectionSeqHiwater: 1183646,
	};
}

// Connect-RPC paths via SvelteKit proxy (/api/v2)
export const KL_GET =
	"**/api/v2/alt.knowledge.loop.v1.KnowledgeLoopService/GetKnowledgeLoop";
export const KL_TRANSITION =
	"**/api/v2/alt.knowledge.loop.v1.KnowledgeLoopService/TransitionKnowledgeLoop";
export const KL_STREAM =
	"**/api/v2/alt.knowledge.loop.v1.KnowledgeLoopService/StreamKnowledgeLoopUpdates";
export const KL_ASK_HANDSHAKE = "**/loop/ask";
