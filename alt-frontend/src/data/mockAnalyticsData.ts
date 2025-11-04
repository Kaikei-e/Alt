// src/data/mockAnalyticsData.ts
import type { ReadingAnalytics, SourceAnalytic, TrendingTopic } from "@/types/analytics";

export const mockAnalytics: ReadingAnalytics = {
  today: {
    articlesRead: 12,
    timeSpent: 45,
    favoriteCount: 3,
    completionRate: 78,
    avgReadingTime: 4.2,
    topCategories: [
      {
        category: "Tech",
        count: 8,
        percentage: 67,
        color: "var(--accent-primary)",
      },
      {
        category: "Design",
        count: 3,
        percentage: 25,
        color: "var(--accent-secondary)",
      },
      {
        category: "Business",
        count: 1,
        percentage: 8,
        color: "var(--accent-tertiary)",
      },
    ],
  },
  week: {
    totalArticles: 67,
    totalTime: 245,
    dailyBreakdown: [
      { day: "Mon", articles: 8, timeSpent: 32, completion: 75 },
      { day: "Tue", articles: 12, timeSpent: 45, completion: 78 },
      { day: "Wed", articles: 15, timeSpent: 52, completion: 82 },
      { day: "Thu", articles: 10, timeSpent: 38, completion: 70 },
      { day: "Fri", articles: 14, timeSpent: 48, completion: 80 },
      { day: "Sat", articles: 4, timeSpent: 15, completion: 90 },
      { day: "Sun", articles: 4, timeSpent: 15, completion: 85 },
    ],
    trendDirection: "up",
    weekOverWeek: 15,
  },
  month: {
    totalArticles: 234,
    totalTime: 1024,
    monthlyGoal: 300,
    progress: 78,
  },
};

export const mockTrendingTopics: TrendingTopic[] = [
  {
    tag: "AI",
    count: 45,
    trend: "up",
    trendValue: 23,
    category: "tech",
    color: "var(--accent-primary)",
  },
  {
    tag: "React",
    count: 32,
    trend: "up",
    trendValue: 12,
    category: "development",
    color: "var(--accent-secondary)",
  },
  {
    tag: "Design",
    count: 28,
    trend: "stable",
    trendValue: 0,
    category: "design",
    color: "var(--accent-tertiary)",
  },
  {
    tag: "Startup",
    count: 19,
    trend: "down",
    trendValue: -8,
    category: "business",
    color: "var(--alt-warning)",
  },
  {
    tag: "Web3",
    count: 15,
    trend: "up",
    trendValue: 34,
    category: "tech",
    color: "var(--alt-success)",
  },
];

export const mockSourceAnalytics: SourceAnalytic[] = [
  {
    id: "techcrunch",
    name: "TechCrunch",
    icon: "📰",
    unreadCount: 12,
    totalArticles: 145,
    avgReadingTime: 4.2,
    reliability: 9.2,
    lastUpdate: "2024-01-15T10:30:00Z",
    engagement: 89,
    category: "tech",
  },
  {
    id: "devto",
    name: "Dev.to",
    icon: "💻",
    unreadCount: 6,
    totalArticles: 98,
    avgReadingTime: 6.8,
    reliability: 8.7,
    lastUpdate: "2024-01-15T09:15:00Z",
    engagement: 76,
    category: "development",
  },
  {
    id: "medium",
    name: "Medium",
    icon: "📝",
    unreadCount: 15,
    totalArticles: 234,
    avgReadingTime: 5.1,
    reliability: 8.1,
    lastUpdate: "2024-01-15T08:45:00Z",
    engagement: 68,
    category: "general",
  },
];
