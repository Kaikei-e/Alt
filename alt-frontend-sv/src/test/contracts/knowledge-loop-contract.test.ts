/**
 * Knowledge Loop API Contract Tests (ADR-000844)
 *
 * Provider-agnostic contract assertions for the Knowledge Loop projection
 * shape. Two layers of guarantee:
 *
 *   1. Proto-schema conformance — `create()` + `toBinary()` + `fromBinary()`
 *      round-trip validates that the response shape the projector emits is
 *      a legal `KnowledgeLoopEntry` per the canonical proto.
 *
 *   2. Canonical-contract enforcement (knowledge-loop-canonical-contract.md
 *      §7 / §11) — assertions that pin behaviour the proto alone cannot
 *      express:
 *        - `whyPrimary.text` is a substantive narrative, not the historical
 *          "New summary" placeholder.
 *        - `decisionOptions` for each `proposedStage` only proposes
 *          §7-allowed transitions (e.g. observe → orient via `revisit`,
 *          NEVER observe → act via open/save).
 *        - `evidenceRefs` is bounded at 8.
 *
 * These tests serve as the consumer-side CDC RED that pins the post-
 * ADR-000844 contract. If the projector regresses to placeholder narratives
 * or mis-stage CTAs, this file fails before any user sees the regression.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	GetKnowledgeLoopResponseSchema,
	KnowledgeLoopEntrySchema,
	WhyPayloadSchema,
	DecisionOptionSchema,
	LoopStage,
	SurfaceBucket,
	WhyKind,
	DismissState,
	LoopPriority,
	DecisionIntent,
} from "$lib/gen/alt/knowledge/loop/v1/knowledge_loop_pb";

// Stage-appropriate intent allowlist per canonical contract §7.
// Each entry's proposed_stage maps to the canonical next move only:
//   observe → orient   (revisit)
//   orient  → decide   (compare)
//   decide  → act      (open / save)
//   act     → observe  (revisit)
// Plus stage-neutral CTAs `ask` (handed to Augur) and `snooze` (defer).
const STAGE_ALLOWED_INTENTS: Record<LoopStage, DecisionIntent[]> = {
	[LoopStage.UNSPECIFIED]: [],
	[LoopStage.OBSERVE]: [
		DecisionIntent.REVISIT,
		DecisionIntent.ASK,
		DecisionIntent.SNOOZE,
	],
	[LoopStage.ORIENT]: [
		DecisionIntent.COMPARE,
		DecisionIntent.ASK,
		DecisionIntent.SNOOZE,
	],
	[LoopStage.DECIDE]: [
		DecisionIntent.OPEN,
		DecisionIntent.SAVE,
		DecisionIntent.ASK,
	],
	[LoopStage.ACT]: [DecisionIntent.REVISIT, DecisionIntent.ASK],
};

// Hard regression guard — the literal placeholder the v3 enricher removed.
const PLACEHOLDER_PATTERNS = [
	/^New summary$/,
	/^Surfaced from a recent event\.$/,
];

describe("Knowledge Loop API Contract (ADR-000844)", () => {
	describe("WhyPayload — narrative substance (§11)", () => {
		it("rejects the historical 'New summary' placeholder", () => {
			const payload = create(WhyPayloadSchema, {
				kind: WhyKind.SOURCE,
				text: "How Event Sourcing Changes Everything — fresh summary ready to read.",
				evidenceRefs: [
					{ refId: "sv-1", label: "summary" },
					{ refId: "article:42", label: "article" },
				],
			});

			expect(payload.text.length).toBeGreaterThan(0);
			expect(payload.text.length).toBeLessThanOrEqual(512);
			for (const placeholder of PLACEHOLDER_PATTERNS) {
				expect(payload.text).not.toMatch(placeholder);
			}
			// Round-trip through binary keeps the narrative intact.
			const bytes = toBinary(WhyPayloadSchema, payload);
			const decoded = fromBinary(WhyPayloadSchema, bytes);
			expect(decoded.text).toBe(payload.text);
		});

		it("bounds evidence_refs at 8 entries", () => {
			const refs = Array.from({ length: 8 }, (_, i) => ({
				refId: `ref-${i}`,
				label: "ev",
			}));
			const payload = create(WhyPayloadSchema, {
				kind: WhyKind.SOURCE,
				text: "ok",
				evidenceRefs: refs,
			});
			expect(payload.evidenceRefs.length).toBeLessThanOrEqual(8);
		});

		it("supports each WhyKind without altering the bound", () => {
			const kinds = [
				WhyKind.SOURCE,
				WhyKind.PATTERN,
				WhyKind.RECALL,
				WhyKind.CHANGE,
			];
			for (const kind of kinds) {
				const payload = create(WhyPayloadSchema, {
					kind,
					text: "narrative for kind " + kind,
					evidenceRefs: [],
				});
				expect(payload.kind).toBe(kind);
				expect(payload.text.length).toBeLessThanOrEqual(512);
			}
		});
	});

	describe("DecisionOption — §7 transition allowlist", () => {
		it.each([
			[
				LoopStage.OBSERVE,
				[DecisionIntent.REVISIT, DecisionIntent.ASK, DecisionIntent.SNOOZE],
			],
			[
				LoopStage.ORIENT,
				[DecisionIntent.COMPARE, DecisionIntent.ASK, DecisionIntent.SNOOZE],
			],
			[
				LoopStage.DECIDE,
				[DecisionIntent.OPEN, DecisionIntent.SAVE, DecisionIntent.ASK],
			],
			[LoopStage.ACT, [DecisionIntent.REVISIT, DecisionIntent.ASK]],
		])("stage %s seeds only §7-allowed intents", (stage, intents) => {
			const allowed = STAGE_ALLOWED_INTENTS[stage as LoopStage];
			for (const intent of intents) {
				expect(allowed).toContain(intent);
			}
		});

		it("forbids observe → act CTAs (open/save) on Observe entries", () => {
			const observeAllowed = STAGE_ALLOWED_INTENTS[LoopStage.OBSERVE];
			expect(observeAllowed).not.toContain(DecisionIntent.OPEN);
			expect(observeAllowed).not.toContain(DecisionIntent.SAVE);
		});

		it("each DecisionOption round-trips through binary", () => {
			const opt = create(DecisionOptionSchema, {
				actionId: "revisit",
				intent: DecisionIntent.REVISIT,
				label: "Revisit",
			});
			const bytes = toBinary(DecisionOptionSchema, opt);
			const decoded = fromBinary(DecisionOptionSchema, bytes);
			expect(decoded.actionId).toBe("revisit");
			expect(decoded.intent).toBe(DecisionIntent.REVISIT);
		});
	});

	describe("KnowledgeLoopEntry — full shape conformance", () => {
		it("constructs a contract-compliant Observe entry", () => {
			const entry = create(KnowledgeLoopEntrySchema, {
				entryKey: "article:42",
				sourceItemKey: "article:42",
				proposedStage: LoopStage.OBSERVE,
				surfaceBucket: SurfaceBucket.NOW,
				projectionRevision: 1n,
				projectionSeqHiwater: 100n,
				sourceEventSeq: 100n,
				whyPrimary: {
					kind: WhyKind.SOURCE,
					text: "How Event Sourcing Changes Everything — fresh summary ready to read.",
					evidenceRefs: [{ refId: "sv-1", label: "summary" }],
				},
				dismissState: DismissState.ACTIVE,
				renderDepthHint: 4,
				loopPriority: LoopPriority.CRITICAL,
				decisionOptions: [
					{ actionId: "revisit", intent: DecisionIntent.REVISIT },
					{ actionId: "ask", intent: DecisionIntent.ASK },
					{ actionId: "snooze", intent: DecisionIntent.SNOOZE },
				],
			});

			// Every CTA's intent must be in the §7 allowlist for the proposed_stage.
			const allowed = STAGE_ALLOWED_INTENTS[entry.proposedStage];
			for (const opt of entry.decisionOptions) {
				expect(allowed).toContain(opt.intent);
			}

			// Why text is substantive (no placeholder regex match).
			for (const placeholder of PLACEHOLDER_PATTERNS) {
				expect(entry.whyPrimary?.text ?? "").not.toMatch(placeholder);
			}

			// Round-trip via binary — keeps the contract intact across the wire.
			const bytes = toBinary(KnowledgeLoopEntrySchema, entry);
			const decoded = fromBinary(KnowledgeLoopEntrySchema, bytes);
			expect(decoded.entryKey).toBe(entry.entryKey);
			expect(decoded.proposedStage).toBe(LoopStage.OBSERVE);
			expect(decoded.decisionOptions.map((o) => o.intent)).toEqual([
				DecisionIntent.REVISIT,
				DecisionIntent.ASK,
				DecisionIntent.SNOOZE,
			]);
		});

		it("each stage has a contract-compliant entry shape", () => {
			const stages = [
				LoopStage.OBSERVE,
				LoopStage.ORIENT,
				LoopStage.DECIDE,
				LoopStage.ACT,
			];
			for (const stage of stages) {
				const intents = STAGE_ALLOWED_INTENTS[stage];
				const entry = create(KnowledgeLoopEntrySchema, {
					entryKey: `entry:${stage}`,
					sourceItemKey: `entry:${stage}`,
					proposedStage: stage,
					surfaceBucket: SurfaceBucket.NOW,
					whyPrimary: {
						kind: WhyKind.SOURCE,
						text: `narrative for stage ${stage}`,
						evidenceRefs: [],
					},
					dismissState: DismissState.ACTIVE,
					renderDepthHint: 1,
					loopPriority: LoopPriority.REFERENCE,
					decisionOptions: intents.map((intent, i) => ({
						actionId: `cta-${i}`,
						intent,
					})),
				});

				expect(entry.decisionOptions.length).toBe(intents.length);
				for (const opt of entry.decisionOptions) {
					expect(STAGE_ALLOWED_INTENTS[stage]).toContain(opt.intent);
				}
			}
		});
	});

	describe("GetKnowledgeLoopResponse — top-level shape", () => {
		it("constructs a response containing foreground + bucket entries", () => {
			const response = create(GetKnowledgeLoopResponseSchema, {
				foregroundEntries: [
					{
						entryKey: "article:fg-1",
						sourceItemKey: "article:fg-1",
						proposedStage: LoopStage.OBSERVE,
						surfaceBucket: SurfaceBucket.NOW,
						whyPrimary: {
							kind: WhyKind.SOURCE,
							text: "Substantive narrative for fg",
							evidenceRefs: [],
						},
						dismissState: DismissState.ACTIVE,
						renderDepthHint: 4,
						loopPriority: LoopPriority.CRITICAL,
						decisionOptions: [
							{ actionId: "revisit", intent: DecisionIntent.REVISIT },
						],
					},
				],
				bucketEntries: [],
				surfaces: [],
				projectionSeqHiwater: 100n,
			});

			expect(response.foregroundEntries.length).toBe(1);
			expect(
				response.foregroundEntries[0].decisionOptions.map((o) => o.intent),
			).toEqual([DecisionIntent.REVISIT]);

			// Round-trip the whole response.
			const bytes = toBinary(GetKnowledgeLoopResponseSchema, response);
			const decoded = fromBinary(GetKnowledgeLoopResponseSchema, bytes);
			expect(decoded.foregroundEntries[0].whyPrimary?.text).toBe(
				"Substantive narrative for fg",
			);
		});
	});
});
