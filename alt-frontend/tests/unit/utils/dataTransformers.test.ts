import { describe, it, expect } from "vitest";
import { transformFeedStats } from '../../../src/dataTransformers";
import { FeedStatsSummary } from "@/schema/feedStats";
import { Rss, Eye, Clock } from "lucide-react";

describe("dataTransformers", () => {
  describe("transformFeedStats", () => {
    it("should transform feed stats to stats card data", () => {
      const mockFeedStats: FeedStatsSummary = {
        feed_amount: { amount: 24 },
        summarized_feed: { amount: 18 },
      };
      const unreadCount = 156;

      const result = transformFeedStats(mockFeedStats, unreadCount);

      expect(result).toHaveLength(3);

      // Check total feeds card
      expect(result[0]).toEqual({
        id: "total-feeds",
        icon: Rss,
        label: "Total Feeds",
        value: 24,
        trend: "+12%",
        trendLabel: "vs last month",
        color: "primary",
      });

      // Check unread articles card
      expect(result[1]).toEqual({
        id: "unread-articles",
        icon: Eye,
        label: "Unread Articles",
        value: 156,
        trend: "+5%",
        trendLabel: "vs yesterday",
        color: "secondary",
      });

      // Check reading time card
      expect(result[2]).toEqual({
        id: "reading-time",
        icon: Clock,
        label: "Reading Time",
        value: 45,
        trend: "+2h",
        trendLabel: "this week",
        color: "tertiary",
      });
    });

    it("should handle null or undefined feed stats", () => {
      const result = transformFeedStats(null, 156);

      expect(result).toHaveLength(3);
      expect(result[0].value).toBe(0); // Default value for total feeds
      expect(result[1].value).toBe(156); // Unread count should still work
    });

    it("should handle missing feed amount data", () => {
      const mockFeedStats = {
        feed_amount: undefined,
        summarized_feed: { amount: 18 },
      } as unknown as FeedStatsSummary;

      const result = transformFeedStats(mockFeedStats, 156);

      expect(result[0].value).toBe(0); // Should default to 0
    });

    it("should handle zero values correctly", () => {
      const mockFeedStats: FeedStatsSummary = {
        feed_amount: { amount: 0 },
        summarized_feed: { amount: 0 },
      };

      const result = transformFeedStats(mockFeedStats, 0);

      expect(result[0].value).toBe(0);
      expect(result[1].value).toBe(0);
      expect(result[2].value).toBe(45); // Reading time should have mock value
    });

    it("should have consistent data structure", () => {
      const mockFeedStats: FeedStatsSummary = {
        feed_amount: { amount: 24 },
        summarized_feed: { amount: 18 },
      };

      const result = transformFeedStats(mockFeedStats, 156);

      result.forEach((card) => {
        expect(card).toHaveProperty("id");
        expect(card).toHaveProperty("icon");
        expect(card).toHaveProperty("label");
        expect(card).toHaveProperty("value");
        expect(card).toHaveProperty("trend");
        expect(card).toHaveProperty("trendLabel");
        expect(card).toHaveProperty("color");
        expect(typeof card.value).toBe("number");
        expect(typeof card.id).toBe("string");
        expect(typeof card.label).toBe("string");
      });
    });
  });
});
