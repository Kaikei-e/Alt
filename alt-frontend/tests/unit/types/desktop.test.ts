import { describe, expect, it } from "vitest";
import type {
  ActivityData,
  QuickActionData,
  StatsCardData,
} from "../../../src/types/desktop";

describe("Desktop Types", () => {
  it("should define StatsCardData interface correctly", () => {
    const mockStatsCard: StatsCardData = {
      id: "test-id",
      icon: () => null,
      label: "Test Label",
      value: 42,
      trend: "+5%",
      trendLabel: "vs last week",
      color: "primary",
    };

    expect(mockStatsCard.id).toBe("test-id");
    expect(mockStatsCard.label).toBe("Test Label");
    expect(mockStatsCard.value).toBe(42);
    expect(mockStatsCard.color).toBe("primary");
    expect(typeof mockStatsCard.icon).toBe("function");
  });

  it("should define ActivityData interface correctly", () => {
    const mockActivity: ActivityData = {
      id: 1,
      type: "new_feed",
      title: "Test Activity",
      time: "2 hours ago",
    };

    expect(mockActivity.id).toBe(1);
    expect(mockActivity.type).toBe("new_feed");
    expect(mockActivity.title).toBe("Test Activity");
    expect(mockActivity.time).toBe("2 hours ago");
  });

  it("should define QuickActionData interface correctly", () => {
    const mockQuickAction: QuickActionData = {
      id: 1,
      label: "Test Action",
      icon: () => null,
      href: "/test",
    };

    expect(mockQuickAction.id).toBe(1);
    expect(mockQuickAction.label).toBe("Test Action");
    expect(mockQuickAction.href).toBe("/test");
    expect(typeof mockQuickAction.icon).toBe("function");
  });

  it("should support all activity types", () => {
    const types: ActivityData["type"][] = [
      "new_feed",
      "ai_summary",
      "bookmark",
      "read",
    ];

    types.forEach((type) => {
      const activity: ActivityData = {
        id: 1,
        type,
        title: "Test",
        time: "now",
      };
      expect(activity.type).toBe(type);
    });
  });

  it("should support all stats card colors", () => {
    const colors: StatsCardData["color"][] = [
      "primary",
      "secondary",
      "tertiary",
    ];

    colors.forEach((color) => {
      const stats: StatsCardData = {
        id: "test",
        icon: () => null,
        label: "Test",
        value: 0,
        color,
      };
      expect(stats.color).toBe(color);
    });
  });
});
