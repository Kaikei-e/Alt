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

// ACT-stage scenario fixture: serves a single foreground entry pre-positioned at
// currentEntryStage = ACT so the workspace renders the Open command directly.
// Backend mock switches to this fixture when the request body's lensModeId is
// "e2e-act" (see tests/e2e/infra/handlers/backend.ts).
export const LOOP_FIXTURE_ACT_ENTRY_KEY = "loop-entry-fixture-act-1";
export const LOOP_FIXTURE_ACT_ARTICLE_ID = "article-act-fixture";
export const LOOP_FIXTURE_ACT_SOURCE_URL =
	"https://example.com/loop-act-article";

// Non-NOW bucket fixtures driving the Surface plane tests (PR-L8).
// Each belongs to exactly one bucket so partitioning in /loop/+page.svelte
// is unambiguous.
export const LOOP_FIXTURE_CONTINUE_ENTRY_KEY = "loop-entry-fixture-continue-1";
export const LOOP_FIXTURE_CHANGED_ENTRY_KEY = "loop-entry-fixture-changed-1";
export const LOOP_FIXTURE_CHANGED_NEW_ENTRY_KEY =
	"loop-entry-fixture-changed-2";
export const LOOP_FIXTURE_REVIEW_ENTRY_KEY = "loop-entry-fixture-review-1";

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
			// Stage-appropriate seed per ADR-000844 / canonical contract §7.
			// Observe → orient via revisit; ask + snooze are non-transition CTAs.
			// The pre-ADR seed (open/save/snooze) required observe → act, which
			// §7 forbids — those CTAs rendered as disabled buttons in the UI.
			decisionOptions: [
				{ actionId: "revisit-1", intent: 5, label: "Revisit" }, // REVISIT
				{ actionId: "ask-1", intent: 2, label: "Ask" }, // ASK
				{ actionId: "snooze-1", intent: 6, label: "Snooze" }, // SNOOZE
			],
			actTargets: [
				{
					targetType: 1, // ACT_TARGET_TYPE_ARTICLE
					targetRef: "article-fixture-1",
					route: "/articles/article-fixture-1",
					sourceUrl: LOOP_FIXTURE_SOURCE_URL,
				},
			],
		},
	],
	bucketEntries: [
		// Continue plane — an entry the user was reading but hasn't finished.
		{
			entryKey: LOOP_FIXTURE_CONTINUE_ENTRY_KEY,
			sourceItemKey: "article-fixture-continue",
			proposedStage: 2, // ORIENT
			surfaceBucket: 2, // CONTINUE
			projectionRevision: "2",
			projectionSeqHiwater: "11",
			freshnessAt: NOW_ISO,
			whyPrimary: {
				kind: 1, // SOURCE
				text: "Unfinished read on OODA loop theory.",
				evidenceRefs: [],
			},
			dismissState: 1, // ACTIVE
			renderDepthHint: 2,
			loopPriority: 2, // CONTINUING
			decisionOptions: [],
			actTargets: [
				{
					targetType: 1,
					targetRef: "article-fixture-continue",
					route: "/articles/article-fixture-continue",
					sourceUrl: "https://example.com/loop-continue",
				},
			],
		},
		// Changed plane — a supersede with THEN/NOW content.
		{
			entryKey: LOOP_FIXTURE_CHANGED_ENTRY_KEY,
			sourceItemKey: "article-fixture-changed",
			proposedStage: 1, // OBSERVE
			surfaceBucket: 3, // CHANGED
			projectionRevision: "3",
			projectionSeqHiwater: "12",
			freshnessAt: NOW_ISO,
			whyPrimary: {
				kind: 4, // CHANGE
				text: "A newer version is available.",
				evidenceRefs: [
					{ refId: "sv-old", label: "previous_summary" },
					{ refId: "sv-new", label: "new_summary" },
				],
			},
			dismissState: 1,
			renderDepthHint: 3,
			loopPriority: 3, // CONFIRM
			supersededByEntryKey: LOOP_FIXTURE_CHANGED_NEW_ENTRY_KEY,
			changeSummary: {
				summary: "Model cardinality bumped from 5 to 7 classes.",
				changedFields: ["summary_excerpt"],
				previousEntryKey: "article-fixture-changed-old",
			},
			decisionOptions: [],
			actTargets: [],
		},
		// Review plane — peripheral recall candidate.
		{
			entryKey: LOOP_FIXTURE_REVIEW_ENTRY_KEY,
			sourceItemKey: "article-fixture-review",
			proposedStage: 1, // OBSERVE
			surfaceBucket: 4, // REVIEW
			projectionRevision: "1",
			projectionSeqHiwater: "13",
			freshnessAt: NOW_ISO,
			whyPrimary: {
				kind: 3, // RECALL
				text: "You opened this before.",
				evidenceRefs: [],
			},
			dismissState: 1,
			renderDepthHint: 1,
			loopPriority: 4, // REFERENCE
			decisionOptions: [],
			actTargets: [
				{
					targetType: 1,
					targetRef: "article-fixture-review",
					route: "/articles/article-fixture-review",
					sourceUrl: "https://example.com/loop-review",
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

// ACT-stage scenario: a foreground entry pre-positioned at currentEntryStage =
// ACT so the workspace renders the Open command directly. Used by the e2e spec
// at tests/e2e/desktop/loop/act-open-loads-article.spec.ts via lensModeId
// "e2e-act" — see backend mock handler.
//
// `route` is the production-shape internal SPA path (the projector writes
// "/articles/<article_id>"); `sourceUrl` is the external HTTPS URL the SPA
// reader needs as `?url=`.  Keeping these distinct catches the regression
// where the FE conflated the two and navigated without `?url=`.
export const CONNECT_KNOWLEDGE_LOOP_ACT_RESPONSE = {
	foregroundEntries: [
		{
			entryKey: LOOP_FIXTURE_ACT_ENTRY_KEY,
			sourceItemKey: `article:${LOOP_FIXTURE_ACT_ARTICLE_ID}`,
			proposedStage: 4, // LOOP_STAGE_ACT
			currentEntryStage: 4, // ACT
			currentEntryStageEnteredAt: NOW_ISO,
			surfaceBucket: 1, // SURFACE_BUCKET_NOW
			projectionRevision: "1",
			projectionSeqHiwater: "20",
			freshnessAt: NOW_ISO,
			sourceObservedAt: NOW_ISO,
			whyPrimary: {
				kind: 1, // SOURCE
				text: "Article ready to open from the Act workspace.",
				confidence: 0.9,
				evidenceRefs: [
					{
						refId: LOOP_FIXTURE_ACT_ARTICLE_ID,
						label: "primary source",
					},
				],
			},
			dismissState: 1,
			renderDepthHint: 2,
			loopPriority: 1,
			decisionOptions: [],
			actTargets: [
				{
					targetType: 1, // ARTICLE
					targetRef: LOOP_FIXTURE_ACT_ARTICLE_ID,
					route: `/articles/${LOOP_FIXTURE_ACT_ARTICLE_ID}`,
					sourceUrl: LOOP_FIXTURE_ACT_SOURCE_URL,
				},
			],
		},
	],
	bucketEntries: [],
	surfaces: [
		{
			surfaceBucket: 1, // NOW
			primaryEntryKey: LOOP_FIXTURE_ACT_ENTRY_KEY,
			secondaryEntryKeys: [],
			projectionRevision: "1",
			projectionSeqHiwater: "20",
			freshnessAt: NOW_ISO,
			serviceQuality: 1,
		},
	],
	sessionState: {
		currentStage: 4, // ACT
		currentStageEnteredAt: NOW_ISO,
		foregroundEntryKey: LOOP_FIXTURE_ACT_ENTRY_KEY,
		focusedEntryKey: LOOP_FIXTURE_ACT_ENTRY_KEY,
		projectionRevision: "1",
		projectionSeqHiwater: "20",
	},
	overallServiceQuality: 1,
	generatedAt: NOW_ISO,
	projectionSeqHiwater: "20",
};
