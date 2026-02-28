/**
 * Recap API Contract Tests
 *
 * Validates recap-related proto schema conformance.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	GetSevenDayRecapResponseSchema,
} from "$lib/gen/alt/recap/v2/recap_pb";
import { buildConnectRecapResponse, buildRecapGenre } from "../../../tests/e2e/fixtures/factories";

describe("Recap API Contract", () => {
	it("GetSevenDayRecapResponse conforms to proto schema", () => {
		const mockData = buildConnectRecapResponse();
		const response = create(GetSevenDayRecapResponseSchema, {
			jobId: mockData.jobId,
			executedAt: mockData.executedAt,
			windowStart: mockData.windowStart,
			windowEnd: mockData.windowEnd,
			totalArticles: mockData.totalArticles,
			genres: mockData.genres.map((g) => ({
				genre: g.genre,
				summary: g.summary,
				topTerms: g.topTerms,
				articleCount: g.articleCount,
				clusterCount: g.clusterCount,
				evidenceLinks: g.evidenceLinks.map((e) => ({
					articleId: e.articleId,
					title: e.title,
					sourceUrl: e.sourceUrl,
					publishedAt: e.publishedAt,
					lang: e.lang,
				})),
				bullets: g.bullets,
			})),
		});

		expect(response.genres).toHaveLength(2);
		expect(response.genres[0].genre).toBe("Technology");
		expect(response.totalArticles).toBe(3);
	});

	it("round-trips through proto serialization", () => {
		const original = create(GetSevenDayRecapResponseSchema, {
			jobId: "test-job",
			executedAt: "2025-12-20T12:00:00Z",
			windowStart: "2025-12-13T00:00:00Z",
			windowEnd: "2025-12-20T00:00:00Z",
			totalArticles: 1,
			genres: [
				{
					genre: "Tech",
					summary: "Summary",
					topTerms: ["AI"],
					articleCount: 1,
					clusterCount: 1,
					evidenceLinks: [
						{
							articleId: "a1",
							title: "Article",
							sourceUrl: "https://example.com",
							publishedAt: "2025-12-20T10:00:00Z",
							lang: "en",
						},
					],
					bullets: ["Key point"],
				},
			],
		});

		const binary = toBinary(GetSevenDayRecapResponseSchema, original);
		const deserialized = fromBinary(
			GetSevenDayRecapResponseSchema,
			binary,
		);

		expect(deserialized.genres).toHaveLength(1);
		expect(deserialized.genres[0].genre).toBe("Tech");
		expect(deserialized.totalArticles).toBe(1);
	});

	it("factory creates valid genre structure", () => {
		const genre = buildRecapGenre("Science", 3);

		expect(genre.genre).toBe("Science");
		expect(genre.articleCount).toBe(3);
		expect(genre.evidenceLinks).toHaveLength(3);
		expect(genre.evidenceLinks[0].articleId).toContain("science");
		expect(genre.bullets).toHaveLength(1);
	});
});
