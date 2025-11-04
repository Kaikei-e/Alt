import type { DesktopFeed, FilterState } from "@/types/desktop-feed";

export const mockDesktopFeeds: DesktopFeed[] = [
  {
    id: "1",
    title: "Test Feed 1",
    description: "Test description 1",
    link: "https://example.com/1",
    published: "2023-12-01T00:00:00Z",
    metadata: {
      source: {
        id: "techcrunch",
        name: "TechCrunch",
        icon: "📰",
        reliability: 8.5,
        category: "tech",
        unreadCount: 5,
        avgReadingTime: 5,
      },
      readingTime: 5,
      engagement: {
        // views: 100,    // Removed: SNS element
        // comments: 10,  // Removed: SNS element
        likes: 20,
        bookmarks: 5,
      },
      tags: ["tech", "news"],
      relatedCount: 3,
      publishedAt: "2 hours ago",
      author: "Test Author",
      summary: "Test summary",
      priority: "high",
      category: "tech",
      difficulty: "intermediate",
    },
    isRead: false,
    isFavorited: false,
    isBookmarked: false,
    readingProgress: 0,
  },
  {
    id: "2",
    title: "Test Feed 2",
    description: "Test description 2",
    link: "https://example.com/2",
    published: "2023-12-01T01:00:00Z",
    metadata: {
      source: {
        id: "medium",
        name: "Medium",
        icon: "📝",
        reliability: 7.5,
        category: "general",
        unreadCount: 2,
        avgReadingTime: 7,
      },
      readingTime: 7,
      engagement: {
        // views: 50,     // Removed: SNS element
        // comments: 5,   // Removed: SNS element
        likes: 10,
        bookmarks: 2,
      },
      tags: ["programming"],
      relatedCount: 1,
      publishedAt: "1 hour ago",
      author: "Another Author",
      summary: "Another summary",
      priority: "medium",
      category: "programming",
      difficulty: "beginner",
    },
    isRead: true,
    isFavorited: true,
    isBookmarked: false,
    readingProgress: 100,
  },
  {
    id: "3",
    title: "Test Feed 3",
    description: "Test description 3",
    link: "https://example.com/3",
    published: "2023-12-01T02:00:00Z",
    metadata: {
      source: {
        id: "github",
        name: "GitHub",
        icon: "🐙",
        reliability: 9.0,
        category: "development",
        unreadCount: 10,
        avgReadingTime: 3,
      },
      readingTime: 3,
      engagement: {
        // views: 200,    // Removed: SNS element
        // comments: 25,  // Removed: SNS element
        likes: 50,
        bookmarks: 15,
      },
      tags: ["github", "open-source"],
      relatedCount: 8,
      publishedAt: "Just now",
      author: "Third Author",
      summary: "Third summary",
      priority: "low",
      category: "development",
      difficulty: "advanced",
    },
    isRead: false,
    isFavorited: false,
    isBookmarked: true,
    readingProgress: 25,
  },
];

export const mockFilters: FilterState = {
  readStatus: "all",
  sources: [],
  priority: "all",
  tags: [],
  timeRange: "all",
};
