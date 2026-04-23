/**
 * Knowledge Loop E2E Fixture
 *
 * Shaped for Connect-RPC JSON decoding: enum values as numeric discriminants,
 * int64 fields as strings, Timestamp as RFC-3339 strings. One seeded foreground
 * entry covers the OODA transition UI cases (tile tap, expand, CTAs, dismiss).
 */

const NOW_ISO = "2026-04-23T10:00:00Z";

export const LOOP_FIXTURE_ENTRY_KEY = "loop-entry-fixture-1";
export const LOOP_FIXTURE_SOURCE_URL =
	"https://example.com/loop-source-article";

export const CONNECT_KNOWLEDGE_LOOP_RESPONSE = {
	foregroundEntries: [
		{
			entryKey: LOOP_FIXTURE_ENTRY_KEY,
			sourceItemKey: "article-fixture-1",
			proposedStage: 1, // LOOP_STAGE_OBSERVE
			surfaceBucket: 1, // SURFACE_BUCKET_NOW
			projectionRevision: "1",
			projectionSeqHiwater: "10",
			freshnessAt: NOW_ISO,
			sourceObservedAt: NOW_ISO,
			whyPrimary: {
				kind: 1, // WHY_KIND_SOURCE
				text: "Fresh long-form on OODA loops in knowledge work.",
				confidence: 0.82,
				evidenceRefs: [{ refId: "article-fixture-1", label: "primary source" }],
			},
			dismissState: 1, // DISMISS_STATE_ACTIVE
			renderDepthHint: 2, // RENDER_DEPTH_HINT_LIGHT
			loopPriority: 1, // LOOP_PRIORITY_CRITICAL
			decisionOptions: [
				{ actionId: "open-1", intent: 1, label: "Open" }, // OPEN
				{ actionId: "ask-1", intent: 2, label: "Ask" }, // ASK (UI filters)
				{ actionId: "save-1", intent: 3, label: "Save" }, // SAVE
				{ actionId: "snooze-1", intent: 6, label: "Snooze" }, // SNOOZE
			],
			actTargets: [
				{
					targetType: 1, // ACT_TARGET_TYPE_ARTICLE
					targetRef: "article-fixture-1",
					route: LOOP_FIXTURE_SOURCE_URL,
				},
			],
		},
	],
	surfaces: [
		{
			surfaceBucket: 1, // NOW
			primaryEntryKey: LOOP_FIXTURE_ENTRY_KEY,
			secondaryEntryKeys: [],
			projectionRevision: "1",
			projectionSeqHiwater: "10",
			freshnessAt: NOW_ISO,
			serviceQuality: 1, // FULL
		},
	],
	sessionState: {
		currentStage: 1, // OBSERVE
		currentStageEnteredAt: NOW_ISO,
		foregroundEntryKey: LOOP_FIXTURE_ENTRY_KEY,
		focusedEntryKey: LOOP_FIXTURE_ENTRY_KEY,
		projectionRevision: "1",
		projectionSeqHiwater: "10",
	},
	overallServiceQuality: 1, // FULL
	generatedAt: NOW_ISO,
	projectionSeqHiwater: "10",
};

export const CONNECT_TRANSITION_LOOP_RESPONSE = {
	accepted: true,
	canonicalEntryKey: LOOP_FIXTURE_ENTRY_KEY,
	message: "",
};
