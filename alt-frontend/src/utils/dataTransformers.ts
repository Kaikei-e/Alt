import { Clock, Eye, Rss } from "lucide-react";
import type { FeedStatsSummary } from "@/schema/feedStats";
import type { StatsCardData } from "@/types/desktop";

/**
 * Transform feed stats and unread count to stats card data format
 */
export const transformFeedStats = (
  feedStats: FeedStatsSummary | null,
  unreadCount: number,
): StatsCardData[] => {
  return [
    {
      id: "total-feeds",
      icon: Rss,
      label: "Total Feeds",
      value: feedStats?.feed_amount?.amount || 0,
      trend: "+12%",
      trendLabel: "vs last month",
      color: "primary",
    },
    {
      id: "unread-articles",
      icon: Eye,
      label: "Unread Articles",
      value: unreadCount,
      trend: "+5%",
      trendLabel: "vs yesterday",
      color: "secondary",
    },
    {
      id: "reading-time",
      icon: Clock,
      label: "Reading Time",
      value: 45, // Mock value - in real implementation this would be calculated
      trend: "+2h",
      trendLabel: "this week",
      color: "tertiary",
    },
  ];
};
