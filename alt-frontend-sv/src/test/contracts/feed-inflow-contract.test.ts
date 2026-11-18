import {
	fromBinary,
	fromJson,
	type JsonObject,
	toBinary,
	toJson,
} from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
	GetFeedInflowStatsResponseSchema,
	SetSubscriptionDailyCapResponseSchema,
} from "#lib/gen/alt/feeds/v2/feeds_pb.js";

describe("FeedInflow Contract", () => {
	it("decodes GetFeedInflowStatsResponse from Connect JSON wire format (int64 as string)", () => {
		const wire: JsonObject = {
			stats: [
				{
					feedLinkId: "11111111-1111-4111-8111-111111111111",
					url: "https://example.com/feed.xml",
					dailyCap: 15,
					inflowCount: "420",
					readCount: "63",
					openedCountLegacy: "12",
					readRate: 0.15,
				},
				{
					feedLinkId: "22222222-2222-4222-8222-222222222222",
					url: "https://quiet.example.org/rss",
					inflowCount: "0",
					readCount: "0",
					openedCountLegacy: "0",
					readRate: 0,
				},
			],
			windowDays: 30,
		};

		const decoded = fromJson(GetFeedInflowStatsResponseSchema, wire);
		expect(decoded.windowDays).toBe(30);
		expect(decoded.stats).toHaveLength(2);
		expect(decoded.stats[0]?.inflowCount).toBe(420n);
		expect(decoded.stats[0]?.dailyCap).toBe(15);
		expect(decoded.stats[1]?.dailyCap).toBeUndefined();
	});

	it("round-trips SetSubscriptionDailyCapResponse with and without dailyCap", () => {
		const setDecoded = fromJson(SetSubscriptionDailyCapResponseSchema, {
			message: "Daily cap updated",
			dailyCap: 20,
		});
		const setRoundTripped = fromBinary(
			SetSubscriptionDailyCapResponseSchema,
			toBinary(SetSubscriptionDailyCapResponseSchema, setDecoded),
		);
		expect(
			toJson(SetSubscriptionDailyCapResponseSchema, setRoundTripped),
		).toEqual({
			message: "Daily cap updated",
			dailyCap: 20,
		});

		const clearDecoded = fromJson(SetSubscriptionDailyCapResponseSchema, {
			message: "Daily cap cleared",
		});
		expect(clearDecoded.dailyCap).toBeUndefined();
	});
});
