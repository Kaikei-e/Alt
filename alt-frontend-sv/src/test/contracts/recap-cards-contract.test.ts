/**
 * Three-Day Topic Recap Cards Contract Tests
 *
 * Validates GetThreeDayRecapCardsResponse proto schema conformance,
 * including nullability of why_ja, genre, continues_card_id, pub_date,
 * and empty job state.
 */

import {
	create,
	fromBinary,
	fromJson,
	toBinary,
	toJson,
} from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
	GetThreeDayRecapCardsResponseSchema,
	RecapCardSchema,
	RecapCardSourceSchema,
	RecapCardsJobSchema,
} from "$lib/gen/alt/recap/v2/recap_pb";
import {
	buildMockDegradedRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../../tests/e2e/fixtures/factories";

describe("Three-Day Recap Cards Contract", () => {
	it("GetThreeDayRecapCardsResponse conforms to proto schema when fully populated", () => {
		const mockData = buildMockRecapCardsResponse();
		if (!mockData.job) {
			throw new Error("mockData.job is required for this test");
		}
		const response = create(GetThreeDayRecapCardsResponseSchema, {
			job: create(RecapCardsJobSchema, {
				jobId: mockData.job.jobId,
				kickedAt: mockData.job.kickedAt,
				from: mockData.job.from,
				to: mockData.job.to,
				paramsVersion: mockData.job.paramsVersion,
				cardsSelected: mockData.job.cardsSelected,
				degraded: mockData.job.degraded,
			}),
			cards: mockData.cards.map((c) =>
				create(RecapCardSchema, {
					id: c.id,
					rank: c.rank,
					storyId: c.storyId,
					continuesCardId: c.continuesCardId ?? undefined,
					headlineJa: c.headlineJa,
					whatJa: c.whatJa,
					whyJa: c.whyJa ?? undefined,
					genre: c.genre ?? undefined,
					createdAt: c.createdAt,
					sources: c.sources.map((s) =>
						create(RecapCardSourceSchema, {
							n: s.n,
							feedId: s.feedId,
							url: s.url,
							host: s.host,
							title: s.title,
							pubDate: s.pubDate ?? undefined,
						}),
					),
				}),
			),
		});

		expect(response.job).toBeDefined();
		if (!response.job) {
			throw new Error("response.job is undefined");
		}
		expect(response.job.jobId).toBe("33333333-3333-3333-3333-333333333333");
		expect(response.job.cardsSelected).toBe(3);
		expect(response.job.degraded).toBe(false);
		expect(response.cards).toHaveLength(3);
		expect(response.cards[0]?.headlineJa).toBe(
			"大規模推論モデルの国内展開が加速",
		);
		expect(response.cards[0]?.sources).toHaveLength(2);
	});

	it("handles nullability of why_ja, genre, continues_card_id, and pub_date", () => {
		// Card with all optional fields omitted
		const card = create(RecapCardSchema, {
			id: "card-minimal",
			rank: 1,
			storyId: "story-1",
			headlineJa: "必要最小限のカード",
			whatJa: "出来事の概要です。[1]",
			createdAt: "2026-09-22T17:00:00Z",
			sources: [
				create(RecapCardSourceSchema, {
					n: 1,
					feedId: "feed-1",
					url: "https://example.com/item",
					host: "example.com",
					title: "Source Title",
				}),
			],
		});

		expect(card.continuesCardId).toBeUndefined();
		expect(card.whyJa).toBeUndefined();
		expect(card.genre).toBeUndefined();
		expect(card.sources[0]?.pubDate).toBeUndefined();

		const response = create(GetThreeDayRecapCardsResponseSchema, {
			cards: [card],
		});

		expect(response.job).toBeUndefined();
		expect(response.cards).toHaveLength(1);
	});

	it("round-trips through binary serialization", () => {
		const mockData = buildMockRecapCardsResponse();
		if (!mockData.job) {
			throw new Error("mockData.job is required for this test");
		}
		const original = create(GetThreeDayRecapCardsResponseSchema, {
			job: create(RecapCardsJobSchema, {
				jobId: mockData.job.jobId,
				kickedAt: mockData.job.kickedAt,
				from: mockData.job.from,
				to: mockData.job.to,
				paramsVersion: mockData.job.paramsVersion,
				cardsSelected: mockData.job.cardsSelected,
				degraded: mockData.job.degraded,
			}),
			cards: mockData.cards.map((c) =>
				create(RecapCardSchema, {
					id: c.id,
					rank: c.rank,
					storyId: c.storyId,
					continuesCardId: c.continuesCardId ?? undefined,
					headlineJa: c.headlineJa,
					whatJa: c.whatJa,
					whyJa: c.whyJa ?? undefined,
					genre: c.genre ?? undefined,
					createdAt: c.createdAt,
					sources: c.sources.map((s) =>
						create(RecapCardSourceSchema, {
							n: s.n,
							feedId: s.feedId,
							url: s.url,
							host: s.host,
							title: s.title,
							pubDate: s.pubDate ?? undefined,
						}),
					),
				}),
			),
		});

		const binary = toBinary(GetThreeDayRecapCardsResponseSchema, original);
		const deserialized = fromBinary(
			GetThreeDayRecapCardsResponseSchema,
			binary,
		);

		expect(deserialized.job?.jobId).toBe(original.job?.jobId);
		expect(deserialized.cards).toHaveLength(3);
		expect(deserialized.cards[1]?.continuesCardId).toBe(
			"11111111-0000-0000-0000-000000000001",
		);
		expect(deserialized.cards[1]?.whyJa).toBeUndefined();
		expect(deserialized.cards[2]?.genre).toBeUndefined();
	});

	it("round-trips through JSON serialization", () => {
		const mockData = buildMockDegradedRecapCardsResponse();
		if (!mockData.job) {
			throw new Error("mockData.job is required for this test");
		}
		const original = create(GetThreeDayRecapCardsResponseSchema, {
			job: create(RecapCardsJobSchema, {
				jobId: mockData.job.jobId,
				kickedAt: mockData.job.kickedAt,
				from: mockData.job.from,
				to: mockData.job.to,
				paramsVersion: mockData.job.paramsVersion,
				cardsSelected: mockData.job.cardsSelected,
				degraded: mockData.job.degraded,
			}),
			cards: mockData.cards.map((c) =>
				create(RecapCardSchema, {
					id: c.id,
					rank: c.rank,
					storyId: c.storyId,
					headlineJa: c.headlineJa,
					whatJa: c.whatJa,
					createdAt: c.createdAt,
					genre: c.genre ?? undefined,
				}),
			),
		});

		const json = toJson(GetThreeDayRecapCardsResponseSchema, original);
		const deserialized = fromJson(GetThreeDayRecapCardsResponseSchema, json);

		expect(deserialized.job?.degraded).toBe(true);
		expect(deserialized.cards[0]?.headlineJa).toBe(
			"データ不足による縮退生成カード",
		);
	});

	it("conforms to empty state when no job exists", () => {
		const empty = create(GetThreeDayRecapCardsResponseSchema, {
			cards: [],
		});

		expect(empty.job).toBeUndefined();
		expect(empty.cards).toHaveLength(0);

		const json = toJson(GetThreeDayRecapCardsResponseSchema, empty);
		const deserialized = fromJson(GetThreeDayRecapCardsResponseSchema, json);
		expect(deserialized.job).toBeUndefined();
		expect(deserialized.cards).toEqual([]);
	});
});
