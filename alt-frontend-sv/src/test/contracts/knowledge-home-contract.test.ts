/**
 * Knowledge Home API Contract Tests
 *
 * Validates Knowledge Home proto schema conformance for both
 * user-facing and admin APIs.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	GetKnowledgeHomeResponseSchema,
	KnowledgeHomeItemSchema,
	WhyReasonSchema,
	SupersedeInfoSchema,
	StreamKnowledgeHomeUpdatesResponseSchema,
	TrackHomeActionRequestSchema,
	RecallCandidateSchema,
	LensVersionSchema,
	ListLensesResponseSchema,
	TodayDigestSchema,
} from "$lib/gen/alt/knowledge_home/v1/knowledge_home_pb";

describe("Knowledge Home API Contract", () => {
	describe("GetKnowledgeHomeResponse", () => {
		it("conforms to proto schema with required fields", () => {
			const response = create(GetKnowledgeHomeResponseSchema, {
				todayDigest: {
					date: "2026-03-18",
					newArticles: 10,
					summarizedArticles: 8,
					unsummarizedArticles: 2,
					topTags: ["AI", "Rust"],
					weeklyRecapAvailable: true,
					eveningPulseAvailable: false,
					digestFreshness: "fresh",
					lastProjectedAt: "2026-03-18T12:00:00Z",
				},
				items: [
					{
						itemKey: "article:123",
						itemType: "article",
						title: "Test Article",
						publishedAt: "2026-03-18T10:00:00Z",
						tags: ["AI"],
						why: [{ code: "new_unread" }],
						score: 0.95,
					},
				],
				nextCursor: "cursor_abc",
				hasMore: true,
				degradedMode: false,
				generatedAt: "2026-03-18T12:00:00Z",
				featureFlags: [
					{ name: "enable_knowledge_home_page", enabled: true },
				],
				serviceQuality: "full",
			});

			expect(response.todayDigest).toBeDefined();
			expect(response.todayDigest?.digestFreshness).toBe("fresh");
			expect(response.todayDigest?.lastProjectedAt).toBe("2026-03-18T12:00:00Z");
			expect(response.items).toHaveLength(1);
			expect(response.items[0].itemKey).toBe("article:123");
			expect(response.generatedAt).toBeTruthy();
			expect(response.serviceQuality).toBe("full");
		});

		it("handles degraded mode with service_quality field", () => {
			const response = create(GetKnowledgeHomeResponseSchema, {
				todayDigest: {
					date: "2026-03-18",
					newArticles: 0,
					summarizedArticles: 0,
					unsummarizedArticles: 0,
					topTags: [],
				},
				items: [],
				degradedMode: true,
				generatedAt: "2026-03-18T12:00:00Z",
				serviceQuality: "degraded",
			});

			expect(response.degradedMode).toBe(true);
			expect(response.serviceQuality).toBe("degraded");
			expect(response.items).toHaveLength(0);
		});

		it("handles fallback mode", () => {
			const response = create(GetKnowledgeHomeResponseSchema, {
				todayDigest: {
					date: "2026-03-18",
					newArticles: 0,
					summarizedArticles: 0,
					unsummarizedArticles: 0,
					topTags: [],
				},
				items: [],
				degradedMode: true,
				generatedAt: "2026-03-18T12:00:00Z",
				serviceQuality: "fallback",
			});

			expect(response.serviceQuality).toBe("fallback");
		});

		it("round-trips through proto serialization", () => {
			const original = create(GetKnowledgeHomeResponseSchema, {
				todayDigest: {
					date: "2026-03-18",
					newArticles: 5,
					summarizedArticles: 3,
					unsummarizedArticles: 2,
					topTags: ["AI"],
				},
				items: [
					{
						itemKey: "article:456",
						itemType: "article",
						title: "Round-trip Test",
						score: 0.8,
						why: [{ code: "tag_hotspot", tag: "AI" }],
					},
				],
				generatedAt: "2026-03-18T12:00:00Z",
				serviceQuality: "full",
			});

			const binary = toBinary(GetKnowledgeHomeResponseSchema, original);
			const deserialized = fromBinary(
				GetKnowledgeHomeResponseSchema,
				binary,
			);

			expect(deserialized.items).toHaveLength(1);
			expect(deserialized.items[0].title).toBe("Round-trip Test");
			expect(deserialized.serviceQuality).toBe("full");
		});
	});

	describe("KnowledgeHomeItem", () => {
		it("validates all item types", () => {
			const itemTypes = ["article", "recap_anchor", "pulse_anchor"];
			for (const itemType of itemTypes) {
				const item = create(KnowledgeHomeItemSchema, {
					itemKey: `${itemType}:1`,
					itemType,
					title: `Test ${itemType}`,
					score: 0.5,
					why: [{ code: "new_unread" }],
				});
				expect(item.itemType).toBe(itemType);
			}
		});

		it("validates why reason codes", () => {
			const knownCodes = [
				"new_unread",
				"in_weekly_recap",
				"pulse_need_to_know",
				"tag_hotspot",
				"recent_interest_match",
				"related_to_recent_search",
				"summary_completed",
			];
			for (const code of knownCodes) {
				const reason = create(WhyReasonSchema, { code });
				expect(reason.code).toBe(code);
			}
		});

		it("validates supersede states", () => {
			const states = [
				"summary_updated",
				"tags_updated",
				"both_updated",
			];
			for (const state of states) {
				const info = create(SupersedeInfoSchema, {
					state,
					supersededAt: "2026-03-18T12:00:00Z",
				});
				expect(info.state).toBe(state);
			}
		});
	});

	describe("StreamKnowledgeHomeUpdatesResponse", () => {
		it("validates canonical stream event types", () => {
			const eventTypes = [
				"item_added",
				"item_updated",
				"item_removed",
				"digest_changed",
				"recall_changed",
				"stream_expired",
			];
			for (const eventType of eventTypes) {
				const event = create(
					StreamKnowledgeHomeUpdatesResponseSchema,
					{
						eventType,
						occurredAt: "2026-03-18T12:00:00Z",
					},
				);
				expect(event.eventType).toBe(eventType);
			}
		});

		it("allows compatibility-only stream event types during transition", () => {
			const compatibilityEventTypes = ["heartbeat", "fallback_to_unary"];
			for (const eventType of compatibilityEventTypes) {
				const event = create(
					StreamKnowledgeHomeUpdatesResponseSchema,
					{
						eventType,
						occurredAt: "2026-03-18T12:00:00Z",
					},
				);
				expect(event.eventType).toBe(eventType);
			}
		});

		it("includes reconnect_after_ms for terminal events", () => {
			const event = create(
				StreamKnowledgeHomeUpdatesResponseSchema,
				{
					eventType: "stream_expired",
					occurredAt: "2026-03-18T12:00:00Z",
					reconnectAfterMs: 5000,
				},
			);
			expect(event.reconnectAfterMs).toBe(5000);
		});
	});

	describe("TrackHomeActionRequest", () => {
		it("validates action types", () => {
			const actionTypes = [
				"open",
				"dismiss",
				"ask",
				"listen",
				"open_recap",
				"open_search",
			];
			for (const actionType of actionTypes) {
				const req = create(TrackHomeActionRequestSchema, {
					actionType,
					itemKey: "article:1",
				});
				expect(req.actionType).toBe(actionType);
				expect(req.itemKey).toBe("article:1");
			}
		});
	});

	describe("LensVersion", () => {
		it("supports feed-based filtering fields", () => {
			const lensVersion = create(LensVersionSchema, {
				versionId: "version-1",
				tagIds: ["AI"],
				feedIds: ["feed-1"],
				timeWindow: "7d",
				sortMode: "relevance",
			});

			const response = create(ListLensesResponseSchema, {
				lenses: [
					{
						lensId: "lens-1",
						name: "AI Lens",
						currentVersion: lensVersion,
					},
				],
				activeLensId: "lens-1",
			});

			expect(response.activeLensId).toBe("lens-1");
			expect(response.lenses[0]?.currentVersion?.feedIds).toEqual(["feed-1"]);
		});
	});

	describe("RecallCandidate", () => {
		it("conforms to schema with embedded item", () => {
			const candidate = create(RecallCandidateSchema, {
				itemKey: "article:789",
				recallScore: 0.85,
				reasons: [
					{
						type: "recently_searched",
						description: "You searched for related topics",
					},
				],
				firstEligibleAt: "2026-03-17T10:00:00Z",
				nextSuggestAt: "2026-03-18T10:00:00Z",
				item: {
					itemKey: "article:789",
					itemType: "article",
					title: "Recalled Article",
					score: 0.7,
					why: [{ code: "new_unread" }],
				},
			});

			expect(candidate.recallScore).toBe(0.85);
			expect(candidate.reasons).toHaveLength(1);
			expect(candidate.item).toBeDefined();
			expect(candidate.item?.title).toBe("Recalled Article");
		});
	});

	describe("TodayDigest freshness fields", () => {
		it("supports digest_freshness and last_projected_at fields", () => {
			const digest = create(TodayDigestSchema, {
				date: "2026-03-18",
				newArticles: 5,
				summarizedArticles: 3,
				unsummarizedArticles: 2,
				topTags: ["AI"],
				digestFreshness: "fresh",
				lastProjectedAt: "2026-03-18T12:00:00Z",
			});

			expect(digest.digestFreshness).toBe("fresh");
			expect(digest.lastProjectedAt).toBe("2026-03-18T12:00:00Z");
		});

		it("validates all freshness states", () => {
			for (const state of ["fresh", "stale", "unknown"]) {
				const digest = create(TodayDigestSchema, {
					date: "2026-03-18",
					digestFreshness: state,
				});
				expect(digest.digestFreshness).toBe(state);
			}
		});

		it("round-trips freshness fields through serialization", () => {
			const original = create(TodayDigestSchema, {
				date: "2026-03-18",
				digestFreshness: "stale",
				lastProjectedAt: "2026-03-18T11:50:00Z",
				needToKnowCount: 7,
			});

			const binary = toBinary(TodayDigestSchema, original);
			const deserialized = fromBinary(TodayDigestSchema, binary);

			expect(deserialized.digestFreshness).toBe("stale");
			expect(deserialized.lastProjectedAt).toBe("2026-03-18T11:50:00Z");
			expect(deserialized.needToKnowCount).toBe(7);
		});
	});
});
